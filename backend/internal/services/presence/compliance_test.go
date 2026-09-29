package presence

import (
	"context"
	"errors"
	"time"
)

type complianceEvaluatorStub struct {
	eligible bool
	err      error

	calls        int
	lastDriverID string
}

func (s *complianceEvaluatorStub) IsEligible(
	_ context.Context,
	driverID string,
	_ time.Time,
) (bool, error) {
	s.calls++
	s.lastDriverID = driverID

	if s.err != nil {
		return false, s.err
	}

	return s.eligible, nil
}

func eligibleComplianceStub() *complianceEvaluatorStub {
	return &complianceEvaluatorStub{
		eligible: true,
	}
}

func failingComplianceStub() *complianceEvaluatorStub {
	return &complianceEvaluatorStub{
		err: errors.New("compliance repository failure"),
	}
}
