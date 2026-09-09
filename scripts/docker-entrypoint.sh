#!/bin/sh
set -eu

mode="${1:-backend}"
if [ "$#" -gt 0 ]; then
    shift
fi

case "$mode" in
    backend)
        if [ "$#" -eq 0 ]; then
            exec /app/zentrola serve
        fi
        exec /app/zentrola "$@"
        ;;
    web)
        exec /app/zentrola-web "$@"
        ;;
    help|-h|--help)
        echo "Usage: docker run <options> zentrola:latest [backend|web] [arguments...]"
        echo "  backend          Run Zentrola Backend in the foreground (default)"
        echo "  web [arguments]  Run Admin Web; for example: web --api https://api.example.com"
        ;;
    *)
        echo "Unknown container mode: $mode; use backend or web." >&2
        exit 2
        ;;
esac
