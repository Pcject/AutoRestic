#!/bin/sh
set -eu

api_pid=""
nginx_pid=""
shutdown_in_progress=0

terminate_children() {
    if [ "${shutdown_in_progress}" -eq 1 ]; then
        return
    fi

    shutdown_in_progress=1

    if [ -n "${api_pid}" ] && kill -0 "${api_pid}" 2>/dev/null; then
        kill "${api_pid}" 2>/dev/null || true
    fi

    if [ -n "${nginx_pid}" ] && kill -0 "${nginx_pid}" 2>/dev/null; then
        kill "${nginx_pid}" 2>/dev/null || true
    fi
}

forward_signal() {
    terminate_children
}

trap forward_signal INT TERM HUP QUIT

/app/autorestic &
api_pid="$!"

nginx -g 'daemon off;' &
nginx_pid="$!"

exit_status=0

while :; do
    if ! kill -0 "${api_pid}" 2>/dev/null; then
        wait "${api_pid}" || exit_status=$?
        break
    fi

    if ! kill -0 "${nginx_pid}" 2>/dev/null; then
        wait "${nginx_pid}" || exit_status=$?
        break
    fi

    sleep 1
done

terminate_children
wait "${api_pid}" 2>/dev/null || true
wait "${nginx_pid}" 2>/dev/null || true

exit "${exit_status}"
