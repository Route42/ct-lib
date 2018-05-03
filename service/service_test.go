package service_test

import (
	"../service"
	"log"
	"testing"
)

// naive implementation which is not efficient :) just used for testing, you can look at it but it won't help you
func TestNew(t *testing.T) {
	s := service.New("Unit Test") // firstname + lastname please
	var result *service.Result
	for i := 0; i < 50; i++ {
		var err error
		result, err = s.Work()
		if err == nil && result != nil && result.IsValid() {
			// done
			break
		}
	}
	if result == nil {
		t.Error("no results")
	}
	log.Printf("result found %v", result)

	// submit result
	s.SubmitResult(result)

	// wait for done
	s.Wait()
}
