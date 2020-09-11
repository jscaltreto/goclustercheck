# goclustercheck

Microservice that replicates the behavior of https://github.com/olafz/percona-clustercheck. Originally based on
https://github.com/gleamicus/mysqlchk, but this uses the mysql client binary instad of a native go sql connection.
Another difference from the latter scripts is this doesn't get a live status with every request, but instead refreshes
the status on an interval (defaulting to every 5 seconds). Shorter or longer intervals may be specified.

## Building
Build with `go build` and copy the resulting binary somewhere sane (e.g. `/usr/local/sbin`). Don't forget to chmod 0755.

## Unit File
Update aguments appropriately
```
[Unit]
Description=Goclustercheck service

[Service]
ExecStart=/usr/local/sbin/goclustercheck --bindport 9200 --username clustercheck --password password
User=nobody
Group=nobody
Restart=always
RestartSec=5s
```