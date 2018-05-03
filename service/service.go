package service

import (
	"bytes"
	secureRand "crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const dataReceiver = "https://us-central1-r42-development.cloudfunctions.net/code-test-receiver"

type Service struct {
	counter uint64
	secret  string
	done    chan bool

	firstSubmission  int64
	validSubmitCount int64
	candidate        string
	startTime        int64
	numCpu           int
	peakGoRoutines   int64
	peakMem          int64
	submissionMap    map[int64]bool
	submissionMapMux sync.RWMutex
	resultCounter    int64
}

// submit valid result
func (service *Service) SubmitResult(result *Result) {
	if atomic.CompareAndSwapInt64(&service.firstSubmission, 0, 1) {
		// first submission
		log.Println("first submission!")

		// track
		service.sendData(map[string]interface{}{
			"type": "firstSubmission",
		})

		// set start time
		atomic.StoreInt64(&service.firstSubmission, time.Now().Unix())

		// start timer
		go func() {
			timer := time.NewTimer(30 * time.Second)
			<-timer.C

			// done
			service.done <- true
			log.Println("done")
		}()
	}

	// only valid stuff here!
	if !result.isValidResult {
		// track
		service.sendData(map[string]interface{}{
			"type": "invalidResultSubmitted",
		})

		panic("submitted invalid result, call IsValid() first and submit ONLY valid ones")
	}

	// only unique ones
	service.submissionMapMux.Lock()
	_, found := service.submissionMap[result.counter]
	if !found {
		service.submissionMap[result.counter] = true
	}
	service.submissionMapMux.Unlock()

	// count
	if !found {
		atomic.AddInt64(&service.validSubmitCount, 1)
	}
}

// call this to wait for results
func (service *Service) Wait() {
	// track
	service.sendData(map[string]interface{}{
		"type": "startWaiting",
	})

	// wait for done
	<-service.done

	// time spent
	timeDone := time.Now().Unix()
	timeStart := atomic.LoadInt64(&service.firstSubmission)
	timeElapsed := timeDone - timeStart

	// done count
	done := atomic.LoadInt64(&service.validSubmitCount)

	// per second
	numPerSecond := float64(done) / float64(timeElapsed)

	// msg
	msg := fmt.Sprintf("elapsed %d seconds %d item(s) done which is %.2f per second", timeElapsed, done, numPerSecond)
	log.Println(msg)

	// profiler data
	numRoutines := atomic.LoadInt64(&service.peakGoRoutines)
	mem := atomic.LoadInt64(&service.peakMem)

	// send
	service.sendData(map[string]interface{}{
		"type":        "finishedAttempt",
		"msg":         msg,
		"rate":        numPerSecond,
		"done":        done,
		"timeElapsed": timeElapsed,
		"numRoutines": numRoutines,
		"peakMemory":  mem,
	})
}

// try to get result
func (service *Service) Work() (result *Result, err error) {
	newCounterValue := atomic.AddUint64(&service.counter, 1)

	// generate random panics
	if newCounterValue%100 == 0 {
		panic("unexpected critical error while working")
	}

	// bit expensive
	makeExpensive(1000)

	// very slow (which needs timeout pattern)
	if newCounterValue > 200 && rand.Intn(50) == 1 {
		time.Sleep(60 * time.Second)
	}

	// error?
	if newCounterValue%50 == 0 {
		return nil, errors.New("oops something went wrong")
	}

	// invalid results
	if rand.Intn(10) == 1 {
		// not valid
		hasher := sha1.New()
		hasher.Write([]byte(fmt.Sprintf("%d", rand.Int())))
		checksum := hasher.Sum(nil)
		result = &Result{
			A:        rand.Int63(),
			B:        rand.Int63(),
			CheckSum: fmt.Sprintf("%x", checksum),
			service:  service,
		}
		return
	}

	// generate real hash
	hasher := sha1.New()
	a := rand.Int63()
	b := rand.Int63()
	hasher.Write([]byte(fmt.Sprintf("%d%d%s", a, b, service.secret)))
	checksum := hasher.Sum(nil)
	result = &Result{
		A:        a,
		B:        b,
		CheckSum: fmt.Sprintf("%x", checksum),
		service:  service,
		counter:  atomic.AddInt64(&service.resultCounter, 1),
	}
	return
}

type Result struct {
	A        int64
	B        int64
	CheckSum string

	// internal
	service       *Service
	isValidResult bool
	mux           sync.RWMutex
	counter       int64
}

// is the result valid?
func (result *Result) IsValid() bool {
	// expensive..
	makeExpensive(5000)

	// panic?
	if rand.Intn(100) == 1 {
		panic("unexpected critical error in validating")
	}

	// checksum
	hasher := sha1.New()
	hasher.Write([]byte(fmt.Sprintf("%d%d%s", result.A, result.B, result.service.secret)))
	checksum := hasher.Sum(nil)
	if fmt.Sprintf("%x", checksum) != result.CheckSum {
		return false
	}

	// set true
	result.mux.Lock()
	result.isValidResult = true
	result.mux.Unlock()

	return true
}

func (service *Service) sendData(m map[string]interface{}) {
	m["candidate"] = service.candidate
	m["startTime"] = service.startTime
	m["numCpu"] = service.numCpu
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	res, err := http.Post(dataReceiver, "application/json", bytes.NewBuffer(b))
	if err != nil {
		log.Printf("failed to send data %s", err)
		return
	}
	if res.StatusCode != 200 {
		log.Printf("non-200 status %d", res.StatusCode)
	}
}
func (service *Service) startProfiler() {
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for _ = range ticker.C {
			// routines
			numRoutines := int64(runtime.NumGoroutine())
			peakOld := atomic.LoadInt64(&service.peakGoRoutines)
			if numRoutines > peakOld {
				atomic.StoreInt64(&service.peakGoRoutines, numRoutines)
			}

			// memory
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			memUsage := int64(m.Alloc)
			memUsageOld := atomic.LoadInt64(&service.peakMem)
			if memUsage > memUsageOld {
				atomic.StoreInt64(&service.peakMem, memUsage)
			}
		}
	}()
}

func makeExpensive(ms int) {
	// expensive..
	time.Sleep(time.Duration(1+rand.Intn(ms)) * time.Millisecond)
}

func New(candidate string) *Service {
	candidate = strings.TrimSpace(candidate)

	// seed non secure random
	rand.Seed(0)

	// token (which is secure)
	randomBytes := make([]byte, 16)
	secureRand.Read(randomBytes)

	// service
	s := &Service{
		candidate:     candidate,
		secret:        string(randomBytes),
		done:          make(chan bool, 1),
		startTime:     time.Now().Unix(),
		numCpu:        runtime.NumCPU(),
		submissionMap: make(map[int64]bool, 0),
	}

	// profiler
	s.startProfiler()

	// start
	s.sendData(map[string]interface{}{
		"type": "startAttempt",
	})

	// name
	if len(candidate) < 5 || strings.Count(candidate, " ") < 1 {
		panic("please provide your candidate name (e.g. John Doe)")
	}
	return s
}
