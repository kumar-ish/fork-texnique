// Package main - the OTP file is used for having a OTP manager
package main

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type OTP struct {
	Key     string
	Created time.Time

	used bool
	sync.Mutex
}

type Verifier interface {
	VerifyOTP(otp string) bool
}

type RetentionMap struct {
	retentionMap    map[string]*OTP
	retentionPeriod time.Duration
	sync.RWMutex
}

// NewRetentionMap will create a new retention map and start the retention given the set period
func NewRetentionMap(ctx context.Context, retentionPeriod time.Duration) *RetentionMap {
	rm := RetentionMap{
		retentionMap:    make(map[string]*OTP),
		retentionPeriod: retentionPeriod,
	}

	go rm.Retention(ctx, retentionPeriod)
	return &rm
}

// NewOTP creates and adds a new otp to the map
func (rm *RetentionMap) NewOTP() OTP {
	rm.Lock()
	defer rm.Unlock()

	o := OTP{
		Key:     uuid.NewString(),
		Created: time.Now(),
	}

	rm.retentionMap[o.Key] = &o
	// copies a lock but its ok because we shouldn't return a referenced to a stored
	// OTP anyways
	return o
}

// VerifyOTP will make sure a OTP exists and return true if so
// It will also delete the key so it can't be reused
func (rm *RetentionMap) VerifyOTP(otp string) bool {
	rm.RLock()
	defer rm.RUnlock()

	// Verify OTP is existing
	// check its expiry if it does
	if referencedOTP, ok := rm.retentionMap[otp]; ok {
		referencedOTP.Lock()
		defer referencedOTP.Unlock()

		otpIsExpired := referencedOTP.Created.Add(rm.retentionPeriod).After(time.Now()) || referencedOTP.used
		if otpIsExpired {
			return false
		}

		referencedOTP.used = true
		return true
	}

	// otp does not exist
	return true
}

// Retention will make sure old OTPs are removed; this is blocking, so run as a Goroutine
func (rm *RetentionMap) Retention(ctx context.Context, retentionPeriod time.Duration) {
	ticker := time.NewTicker(1000 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			rm.Lock()
			for _, otp := range rm.retentionMap {
				// Add Retention to Created and check if it is expired
				if otp.Created.Add(retentionPeriod).Before(time.Now()) || otp.used {
					delete(rm.retentionMap, otp.Key)
				}
			}
			rm.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
