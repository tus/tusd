#!/bin/bash

set -e
set -o pipefail

echo $@

if [ -z "$2" ]; then
    echo "USAGE: upload.sh SIZE NUMBER [TEMP DIR]"
    exit 1
fi

size="$1"
number="$2"
directory="${3:-/tmp}"
file="${directory}/${size}.bin"

openssl rand -out "$file" "$size"

# Get upload size in bytes
upload_size=$(stat -c "%s" "$file")
echo "Generated file with size: ${upload_size} bytes."

# Create uploads
for i in $(seq 1 $number); do
    # Note: I wanted to use the new feature for extracting header values
    # (https://daniel.haxx.se/blog/2022/03/24/easier-header-picking-with-curl/)
    # but this is not yet available on the current curl version in Alpine Linux.
    upload_urls[${i}]="$(curl -X POST -H 'Tus-Resumable: 1.0.0' -H "Upload-Length: ${upload_size}" --fail --silent -i http://tusd:1080/files/ | grep -i ^Location: | cut -d: -f2- | sed 's/^ *\(.*\).*/\1/' | tr -d '\r')"
done

# Perform the uploads in parallel
for i in $(seq 1 $number); do
    curl -X PATCH -H 'Tus-Resumable: 1.0.0' -H 'Upload-Offset: 0' -H 'Content-Type: application/offset+octet-stream' --data-binary "@${file}" "${upload_urls[${i}]}" &
    pids[${i}]=$!
done

# Wait for all uploads to complete
for pid in ${pids[*]}; do
    wait $pid
done
