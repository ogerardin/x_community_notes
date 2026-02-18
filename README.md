# Searchable X/Twitter Community Notes Database

Community Notes, formerly known as Birdwatch, is a feature on X (formerly Twitter) where contributors can add context
such as fact-checks under a post, see https://x.com/i/communitynotes

Unfortunately X/Twitter does not provide a way to search through the notes, either through a web interface or an API.
This project is intended to fill that gap by providing a searchable database of the notes.

## Architecture

The architecture of the project is as follows:

- PostgreSQL database to store the notes
- PostgREST to provide a RESTful API for the database
- Nginx as a reverse proxy for PostgREST and the web search interface
- A loader script that fetches the notes from the Community Notes downloads page and loads them into the database
- A web search interface (built using AlpineJS) that allows users to search through the notes using full-text search and other filters
- Optionally, Swagger UI for API documentation and Adminer for database management

The project can be run using Docker Compose or a single Docker container.

- For local development, the Docker Compose
  version is recommended as it allows for easier access to the database and API for testing and debugging.
- For deployment, the single Docker container version is more suitable as it simplifies deployment and
  reduces the number of services to manage.

In both methods the database data is persisted between restarts in a Docker volume named `notes-db`.


## Method 1: Docker Compose

In this version, we use Docker Compose to start separate containers for the PostgreSQL database, PostgREST, and Nginx. 
The loader script can be run locally on the host machine, and it will connect to the database container to load the notes.

### Requirements

- Internet connection
- Docker with Compose plug-in
- bash shell with curl, jq, unzip installed

### Start the Docker compose stack

```bash
  docker compose up -d
```

This will start the following services:

- `db`: PostgreSQL database
- `postgrest`: RESTful API server 
- `nginx`: Reverse proxy

### Optional
You may also start the following services:

- `swagger`: Swagger UI for PostgREST API

```bash
  docker compose scale swagger=1  
```

- `adminer`: Web-based database management tool

```bash
  docker compose scale adminer=1
```

### Run the Loader

To load the notes into the database, run the loader script:

```bash
  ./grab_notes.sh
```

### Accessing the note search UI

Open the following URL: http://localhost:8080

### Other Useful URLs

| Service                                            | URL                                                                                |
|----------------------------------------------------|------------------------------------------------------------------------------------|
| PostgREST sample query                             | http://localhost:3000/note?limit=50&summary_ts.fts.Nigeria&select=summary          |
| PostgREST sample query through nginx               | http://localhost:8080/api/note?limit=50                                            |
| Adminer (requires `adminer` container)             | http://localhost:8082/?pgsql=db&username=postgres&db=postgres&ns=public&table=note |
| Postgrest SwaggerUI (requires `swagger` container) | http://localhost:8081                                                              |
| Postgres direct connection                         | localhost:5432                                                                     |


## Method 2: Single Docker container

In this version, we use a single Docker container that runs PostgreSQL, PostgREST, and Nginx. 
The loader script runs inside the container.

### Build and start the Docker container

```bash
  ./build_and_run.sh
```

This will start a container named `x-notes` with all the services running inside it.

### Running the loader

To load the notes into the database, run the loader script inside the container:

```bash
    docker exec -it x-notes /bin/sh -c "cd /home && ./grab_notes.sh"
```

#### Monitoring the loader

While the loader is running, you can monitor the logs of the container:

```bash
    docker logs -f x-notes
```

You can also query this special Postgres COPY Progress Reporting view to see the progress of the loading:

```bash
    docker exec -it x-notes psql -U postgres -d postgres -c "SELECT * FROM pg_stat_progress_copy;"
```

Also accessible via PostgREST API:

```bash
    curl http://localhost:8080/api/pg_stat_progress_copy
```

Monitor the `tuples_processed` field to see how many notes have been loaded. The total number of notes to load can be
known by counting the lines of the notes file (minus 1 for the header):

```bash
    docker exec -it x-notes /bin/sh -c "wc -l /home/data/*.tsv"
```


### Accessing the notes

Open the following URL: http://localhost:8080


## Building and Pushing Multi-Architecture Images

The `build_multi.sh` script builds and pushes multi-architecture Docker images with automatic versioning.

#### Versioning Strategy

Images are tagged based on git release tags:

- **Tagged commit** (e.g., `git tag v1.0.0`): Image is tagged as both `ogerardin/x-notes:1.0.0` and
  `ogerardin/x-notes:latest`
- **Untagged commit**: Image is tagged as `ogerardin/x-notes:latest` only (no version-specific tag)

#### Creating a Version Tag

```bash
# Create and push a semantic version tag
git tag v1.0.0
git push origin v1.0.0

# Then build the image
./build_multi.sh
```

This will build and push images tagged as `ogerardin/x-notes:1.0.0` and `ogerardin/x-notes:latest`.

#### Building Without a Tag

```bash
# Build image with current commit (untagged)
./build_multi.sh
```

This will build and push the image tagged as `ogerardin/x-notes:latest` only.

#### Supported Platforms

The build script targets the following platforms:

- `linux/amd64`
- `linux/arm64`
- `linux/i386`
- `linux/arm/v7`

**Note:** Building multi-architecture images requires Docker Buildx and a compatible builder. The script will create the
builder automatically if it doesn't exist.

## Technical notes

### Fetching the notes data
Community notes are made available as downloadable files on this page: https://x.com/i/communitynotes/download-data. 
As of now, this project only handles the main data file containing the notes themselves ("Notes data"). 
Additional data such as  note ratings, notes status history, user enrollment are currently not loaded.

While this is not documented (and hence could change anytime), the pattern of the notes data file URL is as follows: 
`https://ton.twimg.com/birdwatch-public-data/%Y/%m/%d/notes/notes-00000.zip`
Since the frequency of updates is not documented either, and has been observed to lag several days in the past, 
the loader tries to fetch the latest file by trying to access the URL for the current date, and if it fails, 
going back one day at a time until it finds a file that exists.
It is assumed that all notes fit in a single file. This could also break in the future is the
notes data is split into multiple files (like ratings data).

### Getting the data into PostgreSQL

The structure of the notes data files is described on this page: https://communitynotes.x.com/guide/en/under-the-hood/download-data
Fortunately, the TSV file provided by X/Twitter is already in a format that is compatible with PostgresQL `COPY` command,
provided that the target table has the appropriate structure. This is the fastest way to load large amounts of data 
into Postgres.

The structure of the notes table is defined in `sql/notes_ddl.sql`, which is executed automatically by PostgresQL
at startup when its database is empty. Table column names must match strictly the field names 
in the TSV file (first row). There can be additional columns in the table, 
but all columns in the TSV file must be present in the table.

If the TSV file structure changes in the future, the table structure will need to be updated accordingly. The easiest way 
is to delete the Docker volume containing the database files (named `x-notes-db`), update `sql/notes_ddl.sql` and let 
PostgresQL recreate the database and table from scratch at next start.

### Enabling full-text search
The table currently contains a single extra column `summary_ts` which enables using PostgresQL full-text search 
capabilities. This column is generated (computed) using the `to_tsvector` function, and stored into a tsvector format. 
A GIN index is created on this column to allow for fast full-text search queries, using the `summary_ts` field as search 
vector.
For details, see: https://www.postgresql.org/docs/current/textsearch.html

When querying the database through PostgREST, we use the special PostgREST operator `wfts.` on the `summary_ts` column;
this translates to the `websearch_to_tsquery` PostgresQL function, which allows for a web-style user-friendly search 
syntax. For example, the search `climate change` will search for notes whose summary contains the words
"climate" and "change", in any order, and with some tolerance for variations (e.g., "climate-change" or "climate's change" would also match), while 
the search `"climate change"` (with quotes) will search for the exact phrase "climate change" in the text note.

References:
- [PostgresQL Parsing documents](https://www.postgresql.org/docs/current/textsearch-controls.html)
- [PostgREST Full-Text Search](https://postgrest.org/en/v11/references/api/tables_views.html#fts)


 
## TODO

- schedule the loader to run periodically 
- enable manual triggering of the loader through the web interface
- display the progress of the loader in the web interface


