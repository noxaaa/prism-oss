package service

import (
	"context"

	"github.com/noxaaa/prism-oss/pkg/core/dns"
)

type recordingDNSProvider struct {
	actions     []dns.ApplyRecordInput
	contextErr  error
	hasDeadline bool
	inTx        func() bool
	calledInTx  bool
	err         error
	errOnDelete error
	afterApply  func(dns.ApplyRecordInput)
}

func (provider *recordingDNSProvider) ApplyRecord(ctx context.Context, input dns.ApplyRecordInput) error {
	provider.actions = append(provider.actions, input)
	provider.contextErr = ctx.Err()
	_, provider.hasDeadline = ctx.Deadline()
	if provider.inTx != nil && provider.inTx() {
		provider.calledInTx = true
	}
	if provider.afterApply != nil {
		provider.afterApply(input)
	}
	if len(input.Values) == 0 && provider.errOnDelete != nil {
		return provider.errOnDelete
	}
	return provider.err
}
