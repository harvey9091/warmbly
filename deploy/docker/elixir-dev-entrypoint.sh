#!/bin/sh
# Entrypoint for the elixir-dev container. Keeps deps + the build
# tree in sync with mix.lock on every boot and then hands off to
# `mix phx.server`, which boots Phoenix with code reload enabled.
#
# All commands are cheap when nothing changed:
#   - `mix deps.get` is a no-op when deps/ matches mix.lock
#   - `mix deps.compile` only rebuilds packages whose source actually
#     changed (it diffs the deps/ tree)
set -e

cd /app

mix deps.get
mix deps.compile

exec mix phx.server

