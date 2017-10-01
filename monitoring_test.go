package main

import (
	"testing"
	"time"
)

func Test_failedSLA(t *testing.T) {

	oldTime := 1403253684 // 6/20/2014

	res := reservation{
		App:         "jimtest",
		Component:   "monitor",
		Frequency:   5,
		TimeUnits:   "minutes",
		LastCheckin: int64(oldTime),
	}
	if !FailsSLA(res) {
		t.Fatalf("Should have failed checkin SLA")
	}
}

func Test_goodSLA(t *testing.T) {

	curTime := time.Now().UTC().Unix()
	res := reservation{
		App:         "jimtest",
		Component:   "monitor",
		Frequency:   5,
		TimeUnits:   "minutes",
		LastCheckin: int64(curTime),
	}
	if FailsSLA(res) {
		t.Fatalf("Should not have failed checkin SLA, within 5 minutes")
	}
}
