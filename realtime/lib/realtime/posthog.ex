defmodule Realtime.PostHog do
  @moduledoc """
  PostHog error tracking for the realtime service.

  There is no PostHog dependency here and there does not need to be: an
  `$exception` event is one JSON POST to the capture endpoint, in the shape
  PostHog documents for manual capture, and Finch and Jason are already in the
  tree. That keeps the release's dependency list unchanged.

  It is off unless `POSTHOG_KEY` is configured, which is the self-host default:
  nothing is started, nothing is sent and no host is contacted. Nothing here
  identifies a person either. The distinct id names the process, and person
  profiles are switched off per event, so an exception never creates or updates
  a profile for anybody.
  """

  require Logger

  @finch __MODULE__.Finch
  @tasks __MODULE__.Tasks
  @default_host "https://us.i.posthog.com"
  @capture_path "/i/v0/e/"
  @distinct_id "warmbly-realtime"

  @doc """
  Children to start under the application supervisor, or none when no key is
  configured. The task supervisor is what keeps a send off the caller's process:
  a websocket must never wait on an analytics host.
  """
  def children do
    if enabled?() do
      [{Finch, name: @finch}, {Task.Supervisor, name: @tasks}]
    else
      []
    end
  end

  def enabled?, do: key() != nil

  @doc """
  Reports an exception. `opts` may carry `:stacktrace`, `:extra` and `:tags`,
  matching what the Sentry backend accepts, so a caller reports once and both
  backends understand it.
  """
  def capture_exception(exception, opts \\ []) do
    capture(
      item(exception_type(exception), Exception.message(exception), opts),
      "error",
      opts
    )
  end

  @doc """
  Reports a message with no exception attached. The title is fixed so one
  wording does not become one issue: the text is the description.
  """
  def capture_message(message, opts \\ []) do
    capture(item("message", to_string(message), opts), "info", opts)
  end

  # A frameless stacktrace is left off the item entirely rather than sent as
  # null, which ingestion rejects.
  defp item(type, value, opts) do
    base = %{
      "type" => type,
      "value" => value,
      "mechanism" => %{"handled" => true, "synthetic" => false}
    }

    case stacktrace(opts) do
      nil -> base
      trace -> Map.put(base, "stacktrace", trace)
    end
  end

  defp capture(exception, level, opts) do
    case key() do
      nil ->
        :ok

      key ->
        body =
          Jason.encode!(%{
            "api_key" => key,
            "event" => "$exception",
            "distinct_id" => @distinct_id,
            "properties" => properties(exception, level, opts),
            "timestamp" => DateTime.utc_now() |> DateTime.to_iso8601()
          })

        Task.Supervisor.start_child(@tasks, fn -> post(body) end)
        :ok
    end
  end

  defp properties(exception, level, opts) do
    %{
      "$exception_list" => [exception],
      "$exception_level" => level,
      # No person is created or updated by an exception: the distinct id names
      # a process, not somebody.
      "$process_person_profile" => false,
      # A server's IP is the datacentre's, so geolocating it says nothing.
      "$geoip_disable" => true,
      "service" => "realtime",
      "environment" => System.get_env("APP_ENV", "dev"),
      "release" => System.get_env("WARMBLY_RELEASE", "dev")
    }
    |> Map.merge(flatten(Keyword.get(opts, :tags, %{})))
    |> Map.merge(flatten(Keyword.get(opts, :extra, %{})))
  end

  defp flatten(values) when is_map(values) do
    Map.new(values, fn {key, value} -> {to_string(key), stringify(value)} end)
  end

  defp flatten(_), do: %{}

  # Anything that is not already JSON-encodable is inspected rather than
  # dropped: an unserialisable extra must never be why the report is lost.
  defp stringify(value) when is_binary(value) or is_number(value) or is_boolean(value), do: value
  defp stringify(value) when is_atom(value), do: to_string(value)
  defp stringify(value), do: inspect(value)

  defp stacktrace(opts) do
    case Keyword.get(opts, :stacktrace) do
      entries when is_list(entries) and entries != [] ->
        # PostHog reads frames outermost first, the capture site last, which is
        # the reverse of an Elixir stacktrace.
        %{"type" => "raw", "frames" => entries |> Enum.reverse() |> Enum.map(&frame/1)}

      _ ->
        nil
    end
  end

  defp frame({module, function, arity, location}) do
    %{
      # "custom" is what PostHog calls a frame it is not asked to symbolicate.
      "platform" => "custom",
      "lang" => "elixir",
      "function" => Exception.format_mfa(module, function, arity),
      "filename" => location |> Keyword.get(:file, ~c"") |> to_string(),
      "lineno" => Keyword.get(location, :line, 0),
      "in_app" => in_app?(module),
      "resolved" => true
    }
  end

  defp frame(entry),
    do: %{"platform" => "custom", "lang" => "elixir", "function" => inspect(entry)}

  defp in_app?(module) do
    String.starts_with?(Atom.to_string(module), ["Elixir.Realtime", "Elixir.RealtimeWeb"])
  end

  defp post(body) do
    url = host() <> @capture_path

    case Finch.build(:post, url, [{"content-type", "application/json"}], body)
         |> Finch.request(@finch) do
      {:ok, %Finch.Response{status: status}} when status >= 200 and status < 300 ->
        :ok

      {:ok, %Finch.Response{status: status}} ->
        Logger.warning("PostHog rejected an error report with #{status} (check POSTHOG_KEY)")

      {:error, reason} ->
        Logger.warning("cannot reach the PostHog host #{host()}: #{inspect(reason)}")
    end
  end

  defp exception_type(%module{}), do: inspect(module)
  defp exception_type(_), do: "error"

  defp key do
    case Application.get_env(:realtime, :posthog_key) do
      value when is_binary(value) ->
        case String.trim(value) do
          "" -> nil
          trimmed -> trimmed
        end

      _ ->
        nil
    end
  end

  defp host do
    case Application.get_env(:realtime, :posthog_host) do
      value when is_binary(value) ->
        case value |> String.trim() |> String.trim_trailing("/") do
          "" -> @default_host
          trimmed -> trimmed
        end

      _ ->
        @default_host
    end
  end
end
