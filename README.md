GoTel
=========
Who monitors the monitors?

CrowdStrike Cloud Engineering is releasing GoTel which is an internal monitoring service that aims to ensure scheduled jobs, cronjobs, batch oriented work, or general scheduled tasks are completing successfully and within a set SLA time period.

  - Provides coordinator/worker pattern to ensure one gotel is always operational
  - HTTP based API to enable easy integration
  - Ability to pause checkins during scheduled maintenance periods

Authors/Contributors
----
 * [Jim Plush] - Author/maintainer
 * [Sean Berry] - contributor/reviewer

Overview
----
GoTel is for monitoring scheduled operations. 

Most companies have scheduled reports, cron jobs, backup jobs, random data process tasks,  etc..., various things that are expected to run perfectly but sometimes silently fail. GoTel will let them make a "reservation" which means they have to check in during their allotted time frame or alerts will be sent out to the world. When they run they can "checkin" to GoTel which updates their last check-in time. 

This was born from years of experience with various "cron" type jobs that suddenly stop working because the job they ran had a port blocked on it from a firewall config, data sets grow and something that takes 10 minutes now takes 2 hours, something in the environment changed or a myriad of various other failure conditions occurred. GoTel is for when things need to run and you need an independent monitor in your network that is not locked in to a specific vendor. We've also seen alerting failures from 3rd party vendors that are supposed to give us the warm fuzzy feeling they'll alert us when things stop working.

For a toy example take the case where you have a nightly job that removes old data from a data store.  It’s a simple one liner that runs every night. One day you hit the inflection point and indexes turn useless and your script now does a full table scan. Now your 20 minute data clean up job takes 5 hours and you didn’t know about it as soon as it happened.

GoTel is for when you don't need the overhead of an "enterprise" grade schedule monitor. It's for the microservice world where you have apps running in various languages, platforms and locations.
It's expected that you have two GoTel instances up for redundancy (across datacenters). The coordinator will monitor the worker and the worker will monitor the coordinator to ensure GoTel is always operational and if not that alerts are sent out to avoid silent failures.

GoTel has been running inside [CrowdStrike] and has already caught cases in production of scheduled operations silently failing, reducing customer impact. 

![GoTel Arch](http://jimplush.com/images/gotel.png)

Requirements
-----
0.1 expects a MySQL backend for storing jobs and leader election. Future plugins will allow direct integration with ZooKeeper for leader election but v1 keeps the minimum requirements for easier outside adoption.


Getting Started
----
in MySQL create a "gotel" database


Grab the code:

mkdir -p gotel_github/src

cd gotel_github

export GOPATH=gotel_github

go get github.com/CrowdStrike/gotel

go get github.com/go-sql-driver/mysql

cd into github.com/CrowdStrike/gotel/cmd/gotelweb

./run.sh

navigate to: http://127.0.0.1:8080/reservation



Version
----

0.1

Terminology
-----------

GoTel uses a number of hotel related concepts. It's imagined that a small hotel owner becomes fond of her regular guests and gets sad when they don't checkin when they say they're supposed to. 

* [App] - A general application that's under watch
* [Component] - A sub piece of your app, e.g. job1, job2, etc...
* [Reservation] - When your app starts up or is created you create a placeholder and tell GoTel how often it will checkin
* [Checkin] - Your app completed it's work properly and is telling GoTel everything is A-OK
* [Checkout] - If you want to power down an app you can checkout and GoTel will stop alerting on it
* [Snooze] - If your app is down for maintenance you can "snooze" the job checker to avoid alerts getting fired
* [Alerters] - GoTel allows plugins to be created that can output to various notification systems. SMTP, PagerDuty, etc..

Alerters
----
GoTel allows for configurable alerters to be set so when an application doesn't checkin over it's SLA then we fire off to one or more alert systems.

Currently configured alerts:

####SMTP
 - sends emails to the "notify" parameter of a reservation

To Enable:
edit gotel.gcfg and set enable=true under [smtp]

pass in the flag -GOTEL_SMTP_HOST=10.10.1.1 (or whatever your smtp server address is)

 
####PagerDuty
 - creates a pager duty incident that will alert via SMS when an app/component fails to checkin

API
--------------

```sh
// make a reservation that tells GoTel testapp/requests will complete work every 5 minutes or alert me
// supported time_units currently are seconds,minutes,hours
curl -XPOST 'http://127.0.0.1:8080/reservation' -i -H "Content-type: application/json" -d '
{
  "app": "testapp",
  "component": "requests",
  "notify": "jim@foo.com",
  "frequency": 5,
  "time_units": "minutes",
  "owner": "jim@foo.com"
}
'

// checkin for a reservation to avoid having alerts sent
curl -XPOST 'http://127.0.0.1:8080/checkin' -i -H "Content-type: application/json" -d '
{
  "app": "testapp",
  "component": "requests",
  "notes": "all is well"
}
'

// pause (snooze your wakeup call) a job if you're going down for maintenance or testing
curl -XPOST 'http://127.0.0.1:8080/snooze' -i -H "Content-type: application/json" -d '
{
  "app": "testapp",
  "component": "requests",
  "duration": 10,
  "time_units": "hours"
}
'

// checkout/delete reservation
curl -XPOST 'http://127.0.0.1:8080/checkout' -i -H "Content-type: application/json" -d '
{
  "app": "testapp",
  "component": "requests"
}
'

// view all reservations
curl 'http://127.0.0.1:8080/reservation'
```

##### Configure Config File. Instructions in following file

* cmd/gotelweb/gotel.cfcg



Future ToDos
----
 * ZooKeeper option for leadership election
 * Additional Alerter integrations
 * Adding auth/tls support for SMTP alert
 * Better coordinator/worker monitoring.. make sure jobs are fully processed
 * ability to specify on a reservation which alerters you want to include (potentially useful for debugging)
 * web interface to be able to make reservations through a web ui and view stats
 * ability to set up escalation level. e.g. if this reservation fails then it's a "wake me up" type alert




[CrowdStrike]:http://crowdstrike.com/
[Jim Plush]:http://jimplush.com
[Sean Berry]:http://github.com/schleppy
