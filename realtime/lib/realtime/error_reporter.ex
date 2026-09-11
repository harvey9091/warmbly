defmodule Realtime.ErrorReporter do
  @moduledoc """
  Reports issues to every configured backend and mirrors them to local logs in
  non-prod environments.

  PostHog (`POSTHOG_KEY`) is the default backend and Sentry (`SENTRY_DSN`) is
  still supported; either, both or neither can be configured, and with neither
  a report only reaches the log. Callers pass the same options to both: an
  uninitialised backend drops what it is given.
  """

  require Logger

  alias Realtime.PostHog

  def capture_exception(exception, opts \\ []) do
    if local_logging_enabled?() do
      Logger.error(
        "[issue-local][realtime][exception] #{Exception.message(exception)} opts=#{inspect(opts)}"
      )
    end

    PostHog.capture_exception(exception, opts)
    Sentry.capture_exception(exception, opts)
  end

  def capture_message(message, opts \\ []) do
    if local_logging_enabled?() do
      Logger.error("[issue-local][realtime][message] #{message} opts=#{inspect(opts)}")
    end

    PostHog.capture_message(message, opts)
    Sentry.capture_message(message, opts)
  end

  defp local_logging_enabled? do
    System.get_env("APP_ENV", "dev") != "prod"
  end
end
