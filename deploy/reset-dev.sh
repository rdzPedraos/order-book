#!/usr/bin/env bash
# Recreates the local development environment from scratch: empty topics and
# new databases for every service. It deletes all local data.
#
# Run it whenever a change alters the result of commands already in the log,
# like turning on matching: the engine rebuilds its book by rereading
# orders.commands, and an old command would now produce different events under
# the same ids, while the wallet would answer its old reservations again. Such
# a change needs a new, empty log, never a replay of the old one.
#
# Usage: deploy/reset-dev.sh
set -euo pipefail

compose_file="$(dirname "$0")/docker-compose.yml"

docker compose -f "$compose_file" down --volumes
docker compose -f "$compose_file" up -d --wait postgres redpanda
docker compose -f "$compose_file" up -d
# redpanda-init exits once it created the topics; this fails if it did not.
docker compose -f "$compose_file" wait redpanda-init
