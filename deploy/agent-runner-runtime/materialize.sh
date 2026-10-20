#!/bin/sh
set -eu

# Materialize vendored agent tools from /opt/vendor into /opt/*.
# Codex is required; Claude is optional. Development tools come from the image.
# Each component dir is either an unpacked tree (with bin/) or a single tar.

vendor_root="${1:-/opt/vendor}"

flatten_extract() {
	src="$1"
	dest="$2"
	tmp="$(mktemp -d)"
	case "${src}" in
	*.tar.xz) tar -xJf "${src}" -C "${tmp}" ;;
	*.tar.gz|*.tgz) tar -xzf "${src}" -C "${tmp}" ;;
	*.tar|*.tar.tar) tar -xf "${src}" -C "${tmp}" ;;
	*)
		echo "unsupported archive: ${src}" >&2
		exit 1
		;;
	esac
	set -- "${tmp}"/*
	if [ "$#" -eq 1 ] && [ -d "$1" ]; then
		cp -a "$1"/. "${dest}/"
	else
		cp -a "${tmp}"/. "${dest}/"
	fi
	rm -rf "${tmp}"
}

materialize() {
	name="$1"
	dest="$2"
	required="$3"
	src="${vendor_root}/${name}"
	if [ ! -d "${src}" ]; then
		if [ "${required}" = "1" ]; then
			echo "missing ${src}; put the ${name} package under deploy/agent-runner-runtime/${name}/" >&2
			exit 1
		fi
		echo "optional component ${name} not provided, skipping"
		return 0
	fi
	if [ -d "${src}/bin" ]; then
		mkdir -p "${dest}"
		cp -a "${src}/." "${dest}/"
		return 0
	fi
	archive="$(find "${src}" -maxdepth 1 -type f \( \
		-name '*.tar.xz' -o -name '*.tar.gz' -o -name '*.tgz' -o -name '*.tar.tar' -o -name '*.tar' \
		\) | head -n 1)"
	if [ -n "${archive}" ]; then
		mkdir -p "${dest}"
		flatten_extract "${archive}" "${dest}"
		return 0
	fi
	if [ "${required}" = "1" ]; then
		echo "${src} has neither bin/ nor a tar archive" >&2
		exit 1
	fi
	echo "optional component ${name} is empty, skipping"
}

materialize codex-package /opt/codex 1
materialize claude /opt/claude 0

for name in codex claude; do
	dir="/opt/${name}"
	[ -d "${dir}" ] || continue
	chmod -R a+rX "${dir}"
	if [ -d "${dir}/bin" ]; then
		find "${dir}/bin" -maxdepth 1 -type f -exec chmod a+x {} \;
	fi
done
