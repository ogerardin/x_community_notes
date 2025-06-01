#! /bin/bash

csvformat --version && csvcut --version || { echo "Please install CSVkit https://csvkit.readthedocs.io/en/latest/tutorial/1_getting_started.html#installing-csvkit" ; exit ; }
curl --version || { echo "Please install curl" ; exit ; }
ssconvert --version || { echo "Please install GNUmeric: http://www.gnumeric.org/" ; exit ; }

PARTICIPANT_ID=056B1936908F42285AC8A4E4CD928C9BC3DAD8547FEE39B9323E6413AC6A115F


# Get the current day, month, and year into variables
DAY=$(date +%d)
MONTH=$(date +%m)
YEAR=$(date +%Y)

NOTES_DATA_URL=https://ton.twimg.com/birdwatch-public-data/$YEAR/$MONTH/$DAY/notes/notes-00000.tsv
NOTES_FILE=notes-$YEAR$MONTH$DAY.tsv

if ! [[ -f $NOTES_FILE ]] ; then
  echo "Downloading notes from $NOTES_DATA_URL"
  curl -H 'Accept-encoding: gzip' "$NOTES_DATA_URL" -# | gunzip  > "$NOTES_FILE"
  echo "Downloaded as file://$(realpath "$NOTES_FILE")"
fi
ls -lh "$NOTES_FILE"

echo "Filtering participant $PARTICIPANT_ID"
(head -1 "$NOTES_FILE" ; grep $PARTICIPANT_ID <(tail +2 "$NOTES_FILE") ) | sed 's/&quot;/"/g' > notes-filtered.tsv
echo "Done: file://$(realpath notes-filtered.tsv)"

echo "Converting to CSV"
# -t: input is tab-delimited
# -U 1: quote all
csvformat -t notes-filtered.tsv | csvcut -c 'noteId','tweetId','summary' | csvformat -U 1 > notes-filtered.csv
echo "Done: file://$(realpath notes-filtered.csv)"

echo "Converting to XLSX"
ssconvert notes-filtered.csv notes-filtered.xlsx
echo "Done: file://$(realpath notes-filtered.xlsx)"
