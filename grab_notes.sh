#! /bin/bash

set -e

WORKDIR="data"
FORCE_DOWNLOAD=0

# Portable date functions that work on any Linux system
format_date() {
    local input_date="$1"
    local format="$2"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        date -j -f "%Y-%m-%d" "$input_date" "$format"
    else
        date -d "$input_date" "$format"
    fi
}

get_yesterday() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        date -v-1d "+%Y-%m-%d"
    else
        date -d "yesterday" "+%Y-%m-%d"
    fi
}

mkdir -p "$WORKDIR"

download_notes_file() {
    local date="$1"
    echo "Grabbing notes file for date: $date" >&2
    local filename
    filename="$(format_date "$date" "+%Y-%m-%d-notes-00000.zip")"
    local url
    url="$(format_date "$date" "+https://ton.twimg.com/birdwatch-public-data/%Y/%m/%d/notes/notes-00000.zip")"
    local file="$WORKDIR/$filename"

    if [[ $FORCE_DOWNLOAD -eq 0 && -f "$file" ]]; then
        echo "  file already exists: $file" >&2
        echo "$file"
        return 0
    fi

    echo "  Downloading $url to $file..." >&2
    curl -fSL "$url" -o "$file" && echo "$file"
}

unzip_notes_file() {
    local zipfile="$1"
    local tsvfile="${zipfile%.zip}.tsv"
    echo "Extracting $tsvfile..." >&2
    unzip -p "$zipfile" "notes-00000.tsv" > "$tsvfile"
    echo "$tsvfile"
}

get_container_path() {
    local file="$1"
    local container="$2"
    local filepath="$(realpath "$file")"
    local mounted_file=""

    while read -r source target; do
        if [[ "$filepath" = "$source"* ]]; then
            relpath="${filepath#$source}"
            # Remove leading slash if present
            relpath="${relpath#/}"
            echo "$target/$relpath"; return 0
        fi
    done < <(docker inspect "$container" | jq -r '.[0].Mounts[] | "\(.Source) \(.Destination)"')

    echo "Could not map $file to a container path in container $container" >&2
    return 1
}

load_tsv_to_db() {
    local tsvfile="$1"
    echo "Loading $tsvfile into PostgreSQL..." >&2

    # Find the mount point in the container that matches the local file path
    local mounted_file
    mounted_file=$(get_container_path "$tsvfile" "community_notes-db-1") || return 1
    echo "  mapped $tsvfile to $mounted_file in the container." >&2

    echo "  truncating existing note table..." >&2
    docker exec -it community_notes-db-1 psql -U postgres -c "truncate note;"
    echo "  copying data from $mounted_file..." >&2
    docker exec -it community_notes-db-1 \
      psql -U postgres -c "\copy note FROM '$mounted_file' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)"
}

get_latest_notes_file() {
    local today yesterday zipfile
    today=$(date "+%Y-%m-%d")
    yesterday=$(get_yesterday)

    zipfile=$(download_notes_file "$today")     && { echo "$zipfile"; return 0; }
    zipfile=$(download_notes_file "$yesterday") && { echo "$zipfile"; return 0; }

    echo "Failed to download notes file for $today and $yesterday." >&2
    return 1
}

main() {
    local zipfile tsvfile

    # Step 1: Download and extract the latest notes file
    zipfile=$(get_latest_notes_file)
    tsvfile=$(unzip_notes_file "$zipfile")

    # Step 2: Load the TSV file into the PostgreSQL database
    load_tsv_to_db "$tsvfile"
}

main "$@"

