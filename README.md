# goclustercheck

Microservice that replicates the behavior of https://github.com/olafz/percona-clustercheck. Originally based on
https://github.com/gleamicus/mysqlchk, but this uses the mysql client binary instad of a native go sql connection.
Another difference from the latter scripts is this doesn't get a live status with every request, but instead refreshes
the status on an interval (defaulting to every 5 seconds). Shorter or longer intervals may be specified.


## Building
Build with `go build` and copy the resulting binary somewhere sane (e.g. `/usr/local/sbin`). Don't forget to chmod 0755.

## Usage
The following arguments may be specified:

* --bindaddr [HOSTNAME] - HTTP bind address (default "", listen on all)
* --bindport [PORT] - HTTP bind port (default 9200)
* --checkInterval [DURATION] - How often to check mysql status (default 5s)
* --upfile [FILENAME] - When this file exists, checks always pass (default "/dev/shm/proxyon")
* --failfile [FILENAME] - When this file exists and `upfile` doesn't, checks always fail (default "/dev/shm/proxyoff")
* --mysql-binary [FILENAME] - Path to mysql binary (default "mysql")
* --timeout [DURATION] - Timeout for status checks
* --donor - Available while node is a donor
* --readonly - Available when Read Only

Since this essentially wraps the mysql command, any arguments passed after `--` are passed directly to the mysql command
line. This allows to to specify arguments such as host, port, etc. You can even use a `.cnf` file to avoid having to
store passwords in the clear in unit files.

## Unit File
Update aguments appropriately
```
[Unit]
Description=Goclustercheck service

[Service]
ExecStart=/usr/local/sbin/goclustercheck --bindport 9200 -- -u clustercheck --password=password
User=nobody
Group=nobody
Restart=always
RestartSec=5s
```