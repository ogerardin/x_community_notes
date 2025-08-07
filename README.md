
# Community Notes Database

This project is intended to load X/Twitter Community Notes into a database to allow
quick search and retrieval of the notes.

## Requirements
- Internet connection
- Docker daemon running 

## Loading Notes (Docker Commpose and local script version)
In this version, we use Docker Compose to set up a PostgreSQL database, PostgREST for a RESTful API, and Nginx as a 
reverse proxy. The notes are loaded into the database using a local script that fetches the notes from the 
Community Notes API.

### Additional Requirements
- Docker Compose
- bash shell with curl, jq, unzip installed

### Start the Docker compose stack

```bash
  docker compose up -d
```
This will start the following services:
- `db`: PostgreSQL database
- `postgrest`: RESTful API server for PostgreSQL
- `nginx`: Reverse proxy for PostgREST and web search interface

Optionally you may also start the following services:
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

### Accessing the notes

Open the following URL: http://localhost:8082/alpine.html

### Other Useful URLs

| Service                | URL                                                                                |
|------------------------|------------------------------------------------------------------------------------|
| Postgrest sample query | http://localhost:3000/note?limit=50&summary_ts.fts.Nigeria&select=summary          |
| Adminer                | http://localhost:8080/?pgsql=db&username=postgres&db=postgres&ns=public&table=note |
| Postgrest SwaggerUI    | http://localhost:8081/                                                             |
| Nginx sample query     | http://localhost:8082/api/note?limit=50                                            |


## Loading Notes (Single Docker container version) 
In this version, we use a single Docker container that runs PostgreSQL, PostgREST, and Nginx. The notes are loaded into 
the database using a script that runs inside the container.

### Build and start the Docker container
```bash
  docker build --tag ogerardin/x-notes:latest --file Dockerfile-alpine . && \
  docker run --detach --name x-notes \
      --publish 8080:8080 \
      --mount type=volume,source=x-notes-db,target=/var/lib/postgresql/data \
      ogerardin/x-notes:latest
```

The database data is persisted between container restarts in a Docker volume named `notes-db`.

Optionally you can mount `/home/data` to a local directory to cache the downloaded notes files:
```
  --mount type=bind,source="$(pwd)"/data,target=/home/data \
```

You can also expose the database port (5432) and the PostgREST API port (3000):
```
  --publish 5432:5432 \
  --publish 3000:3000 \
```

### Run the Loader
To load the notes into the database, run the loader script inside the container:
```bash
    docker exec -it x-notes /bin/sh -c "cd /home && ./grab_notes.sh"
```

### Accessing the notes
Open the following URL: http://localhost:8080


