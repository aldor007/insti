# instagram-stats

Very simple application for collecting Instagram stats (likes, comments, followers count) and post scheduler. It is still on 
the early stage of development (frontend required a lot of attention).

## Features
* export stats to csv
* expose stats for Prometheus
* silly post schedule with image crop


## Grafana dashboard
![grafana](doc/grafana.png?raw=true "Title")

## Example uage
```
$ INSTA_USERNAME='user' INSTA_PASSWORD='password'  go run main.go -user USER_TO_OBSERVE -csvPath insta.csv

```
