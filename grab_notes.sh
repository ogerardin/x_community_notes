#! /bin/bash

set -e

DATADIR="data"
FORCE_DOWNLOAD=0

update_import_status() {
    local status="$1"
    local total_rows="${2:-0}"
    local error_message="${3:-}"
    
    local completed_at=""
    if [ "$status" = "completed" ] || [ "$status" = "failed" ]; then
        completed_at=", completed_at=NOW()"
    fi
    
    if [ -n "$error_message" ]; then
        error_msg=", error_message='$error_message'"
    fi
    
    psql -U postgres -c "UPDATE import_status SET status='$status', total_rows=$total_rows$completed_at$error_msg WHERE id=1;" 2>/dev/null || true
}

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

# Return YYYY-MM-DD for N days ago (0 = today). Works on macOS and Linux.
get_date_days_ago() {
    local n="$1"
    if [[ -z "$n" ]]; then n=0; fi
    if [[ "$OSTYPE" == "darwin"* ]]; then
        date -v-"${n}"d "+%Y-%m-%d"
    else
        date -u -I -d "@$(( $(date +%s) - n*86400 ))"
    fi
}

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

    local filepath
    filepath="$(realpath "$file")"
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
    
    local total_rows
    if [ -s "$tsvfile" ]; then
        total_rows=$(($(wc -l < "$tsvfile") - 1))
    else
        total_rows=0
    fi
    update_import_status "running" "$total_rows"

    echo "Loading $tsvfile into local PostgreSQL..." >&2

    # clear the table and copy the data
    echo "  truncating existing note table..." >&2
    psql -U postgres -c "truncate note;"
    echo "  copying data from $tsvfile ($total_rows rows)..." >&2
    if ! psql -U postgres -c "\copy note FROM '$tsvfile' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)"; then
        update_import_status "failed" 0 "Copy failed"
        return 1
    fi
    update_import_status "completed" "$total_rows"
}

# Load the TSV file into the PostgreSQL database running in a Docker container
load_tsv_to_db_docker() {
    local tsvfile="$1"
    local container="$2"

    # Find the mount point in the container that matches the local file path
    local mounted_file
    mounted_file=$(get_container_path "$tsvfile" "$container") || { update_import_status "failed" 0 "Could not map file path"; return 1; }
    echo "  mapped $tsvfile to $mounted_file in container $container." >&2
    
    # Get total row count from local file
    local total_rows
    if [ -s "$tsvfile" ]; then
        total_rows=$(($(wc -l < "$tsvfile") - 1))
    else
        total_rows=0
    fi
    update_import_status "running" "$total_rows"

    echo "Loading $tsvfile into PostgreSQL..." >&2

    # clear the table and copy the data
    echo "  truncating existing note table..." >&2
    docker exec -it "$container" psql -U postgres -c "truncate note;"
    echo "  copying data from $mounted_file ($total_rows rows)..." >&2
    if ! docker exec -it "$container" \
      psql -U postgres -c "\copy note FROM '$mounted_file' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)"; then
        update_import_status "failed" 0 "Copy failed"
        return 1
    fi
    update_import_status "completed" "$total_rows"
}

get_latest_notes_file() {
    # Accept optional first argument: number of days to look back (default 7)
    local lookback_days="${1:-7}"

    # Validate lookback_days is a positive integer
    if ! [[ "$lookback_days" =~ ^[0-9]+$ ]] || [ "$lookback_days" -lt 1 ]; then
        echo "Invalid lookback days: $lookback_days" >&2
        return 1
    fi

    local i date zipfile
    for (( i=0; i<lookback_days; i++ )); do
        date=$(get_date_days_ago "$i")
        zipfile=$(download_notes_file "$date") && { echo "$zipfile"; return 0; }
    done

    echo "Failed to download notes file for last $lookback_days days." >&2
    return 1
}

main() {
    local zipfile tsvfile
    
    trap 'update_import_status "failed" 0 "Script error: $?"' ERR

    # Ensure the data directory exists so downloads succeed
    mkdir -p "$DATADIR"

    # Step 1: Download and extract the latest notes file
    # Optionally accept a first arg for lookback days and pass it to get_latest_notes_file
    zipfile=$(get_latest_notes_file "$1")
    tsvfile=$(unzip_notes_file "$zipfile")

    # Step 2: Load the TSV file into the PostgreSQL database
    if pg_isready; then
        echo "PostgreSQL is running locally" >&2
        load_tsv_to_db "$tsvfile"
    else
        echo "PostgreSQL is not running locally - trying docker container" >&2
        load_tsv_to_db_docker "$tsvfile" "x-notes-db"
    fi
}

main "$@"

