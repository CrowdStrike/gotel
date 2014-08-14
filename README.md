GoTel
=========
Who monitors the monitors?

GoTel is a monitoring service that aims to ensure scheduled jobs, cronjobs, batch oriented work, or general scheduled tasks are completing successfully. 

  - Provides coordinator/worker pattern to ensure one gotel is always operational
  - HTTP based API to enable easy integration
  - Ability to pause checkins during scheduled maintenance periods

Authors/Contributors
----
 * [Jim Plush] - Author/maintainer
 * [Sean Berry] - contributor/reviewer

Overview
----
GoTel is for monitoring scheduled operations. It's expected that you have two GoTel instances up for redundancy. The coordinator will monitor the worker and the worker will monitor the coordinator to ensure GoTel is always operational and if not that alerts are sent out to avoid silent failures.

For example we have monitoring jobs, schedule reports, cron jobs, etc..., various things that are expected to run perfectly and sometimes silently fail. GoTel will let them make a "reservation"
which means they have to check in during their allotted timeframe or alerts will be sent out to the world. When they run they can "checkin" to gotel which updates their last checkin time. 

This was born from years of experience with various "cron" type jobs that suddently stop working because the job they ran suddenly had a port blocked on it from a firewall config, something in the environment changed or a myriad of various other failure conditions occurred. GoTel is for when things need to run and you need an independent monitor in your network that is not locked in to a specific vendor. We've also seen alerting failures from 3rd party vendors that are supposed to give us the warm fuzzy feeling they'll alert us when things stop working.

It's also for when you don't want the overhead of an "enterprise" grade schedule monitor. It's for the microservice world where you have apps running in various languages, platforms and locations.

GoTel has been running inside [CrowdStrike] and has already caught cases in production of scheduled operations silently failing, reducing customer impact. 

![GoTel Arch](http://jimplush.com/images/gotel.png)

Requirements
-----
0.1 expects a MySQL backend for storing jobs and leader election. Future plugins will allow direct integration with ZooKeeper for leader election but v1 keeps the minimum requirements for easier outside adoption.


Getting Started
----
in MySQL create a "gotel" database

cd into cmd/gotelweb

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
* [Pause] - If your app is down for maintenance you can "pause" the job checker to avoid alerts getting fired
* [Alerters] - GoTel allows plugins to be created that can output to various notification systems. SMTP, PagerDuty, etc..

Alerters
----
GoTel allows for configurable alerters to be set so when an application doesn't checkin over it's SLA then we fire off to one or more alert systems.

Currently configured alerts:

SMTP
 - sends emails to the "notify" parameter of a reservation
 
PagerDuty
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

// pause a job if you're going down for maintenance or testing
curl -XPOST 'http://127.0.0.1:8080/pause' -i -H "Content-type: application/json" -d '
{
  "app": "testapp",
  "component": "requests",
  "frequency": 10,
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
[Sean Berry]:http://seanberry.com
