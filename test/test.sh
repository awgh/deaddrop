#!/bin/bash

mkdir -p 1/outbox
mkdir -p 1/inbox

mkdir -p 2/outbox
mkdir -p 2/inbox

cp test.bin 1/outbox/

go build github.com/awgh/deaddrop 
screen -dmS a ./deaddrop -dbfile deaddrop1.ql -p 20001 -rxdir 1/inbox -txdir 1/outbox
screen -dmS b ./deaddrop -dbfile deaddrop2.ql -p 20002 -rxdir 2/inbox -txdir 2/outbox
