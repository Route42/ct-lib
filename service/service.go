package service

import (
	secureRand "crypto/rand"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Service struct {
	counter uint64
	secret  string
	done    chan bool

	firstSubmission  int64
	validSubmitCount int64
	candidate        string
}

// submit valid result
func (service *Service) SubmitResult(result *Result) {
	if atomic.CompareAndSwapInt64(&service.firstSubmission, 0, 1) {
		// first submission
		log.Println("first submission!")

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
		panic("submitted invalid result, call IsValid() first and submit ONLY valid ones")
	}

	// count
	atomic.AddInt64(&service.validSubmitCount, 1)
}

// call this to wait for results
func (service *Service) Wait() {
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

	log.Printf("elapsed %d seconds %d item(s) done which is %.2f per second", timeElapsed, done, numPerSecond)
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
	if newCounterValue > 200 {
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

func makeExpensive(ms int) {
	// expensive..
	time.Sleep(time.Duration(1+rand.Intn(ms)) * time.Millisecond)
}

func New(candidate string) *Service {
	candidate = strings.TrimSpace(candidate)
	if len(candidate) < 5 || strings.Count(candidate, " ") < 1 {
		panic("please provide your candidate name (e.g. John Doe)")
	}
	randomBytes := make([]byte, 16)
	secureRand.Read(randomBytes)
	return &Service{
		candidate: candidate,
		secret:    string(randomBytes),
		done:      make(chan bool, 1),
	}
}
