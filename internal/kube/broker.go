// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package kube

import "sync"

// Event is a change notification for the SSE stream (docs/api.md, GET /api/v1/events).
type Event struct {
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Run        string `json:"run"`
}

const subscriberBuffer = 256

// Broker fans change events out to subscribers. A subscriber that stops reading loses
// events instead of blocking the informers; the SPA treats events as hints to refetch.
type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{subs: map[chan Event]struct{}{}}
}

// Subscribe returns a channel of events and the function that ends the subscription.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Publish sends e to every subscriber that has room for it.
func (b *Broker) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
