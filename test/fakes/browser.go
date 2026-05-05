package fakes

import (
	"context"
	"sync"
)

type FakeBrowser struct {
	mu      sync.Mutex
	Opened  []string
	OpenErr error
}

func NewFakeBrowser() *FakeBrowser { return &FakeBrowser{} }

func (f *FakeBrowser) Open(_ context.Context, url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.OpenErr != nil {
		return f.OpenErr
	}
	f.Opened = append(f.Opened, url)
	return nil
}
