
# Community Notes Database

This project is intended to load X/Twitter Community Notes into a database to allow
quick search and retrieval of the notes.

## Requirements
- internet connection
- Docker with Docker Compose
- bash shell with curl, jq, unzip installed

## Loading Notes

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

## Accessing the notes

Open the following URL: http://localhost:8082/alpine.html

### Other Useful URLs

| Service                | URL                                                                                |
|------------------------|------------------------------------------------------------------------------------|
| Postgrest sample query | http://localhost:3000/note?limit=50&summary_ts.fts.Nigeria&select=summary          |
| Adminer                | http://localhost:8080/?pgsql=db&username=postgres&db=postgres&ns=public&table=note |
| Postgrest SwaggerUI    | http://localhost:8081/                                                             |
| Nginx sample query     | http://localhost:8082/api/note?limit=50                                            |


