#! /bin/bash

set -e

DATADIR="data"
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
    date -v-1d "+%Y-%m-%d" 2>/dev/null || date -u -I -d "@$(( $(date +%s) - 86400 ))"
}

mkdir -p "$DATADIR"

download_notes_file() {
    local date="$1"
    echo "Grabbing notes file for date: $date" >&2
    local filename
    filename="$(format_date "$date" "+%Y-%m-%d-notes-00000.zip")"
    local url
    url="$(format_date "$date" "+https://ton.twimg.com/birdwatch-public-data/%Y/%m/%d/notes/notes-00000.zip")"
    local file="$DATADIR/$filename"

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

# Get the path of a file inside a Docker container based on the local file path
get_container_path() {
    local file="$1"
    local container="$2"

    local filepath="$(realpath "$file")"
    local mounted_file=""

    # iterate over the mounts of the container to find the matching source path
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

# Load the TSV file into the local PostgreSQL database
load_tsv_to_db() {
    local tsvfile="$1"

    echo "Loading $tsvfile into local PostgreSQL..." >&2

    # clear the table and copy the data
    echo "  truncating existing note table..." >&2
    psql -U postgres -c "truncate note;"
    echo "  copying data from $tsvfile..." >&2
    psql -U postgres -c "\copy note FROM '$tsvfile' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)"
}

# Load the TSV file into the PostgreSQL database running in a Docker container
load_tsv_to_db_docker() {
    local tsvfile="$1"
    local container="$2"

    echo "Loading $tsvfile into PostgreSQL..." >&2

    # Find the mount point in the container that matches the local file path
    local mounted_file
    mounted_file=$(get_container_path "$tsvfile" "$container") || return 1
    echo "  mapped $tsvfile to $mounted_file in the container." >&2

    # clear the table and copy the data
    echo "  truncating existing note table..." >&2
    docker exec -it "$container" psql -U postgres -c "truncate note;"
    echo "  copying data from $mounted_file..." >&2
    docker exec -it "$container" \
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
    if pg_isready; then
        echo "PostgreSQL is running locally" >&2
        load_tsv_to_db "$tsvfile"
    else
        echo "PostgreSQL is not running locally - trying docker container" >&2
        load_tsv_to_db_docker "$tsvfile" "community_notes-db-1"
    fi
}

main "$@"

