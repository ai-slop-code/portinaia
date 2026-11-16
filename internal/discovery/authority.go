// Package discovery contains transport-independent rules for interpreting
// collector outcomes. In particular, only an explicit successful collection
// can establish absence; empty successful results remain authoritative.
package discovery

type CollectorOutcome struct {
	Status string
}

func (o CollectorOutcome) Authoritative() bool { return o.Status == "success" }
