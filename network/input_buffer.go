package network

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ErrMissingClientInputID indicates that an input cannot be associated with a client.
	ErrMissingClientInputID = errors.New("missing client input id")
	// ErrStaleClientInput indicates that an input targets a tick outside the configured past window.
	ErrStaleClientInput = errors.New("stale client input")
	// ErrFutureClientInput indicates that an input targets a tick outside the configured future window.
	ErrFutureClientInput = errors.New("future client input")
)

// ClientInput is a buffered, tick-aligned input envelope. Payload is intentionally
// opaque so game code can decide whether it contains a built-in PlayerInput,
// GenericClientInput bytes, or a project-specific decoded value.
type ClientInput struct {
	ClientID   string
	Sequence   uint64
	Tick       uint64
	ObjectType string
	ObjectID   string
	TargetID   string
	Payload    any
	ReceivedAt time.Time
}

// GenericClientInput is the built-in wire message for arbitrary tick-aligned
// input payloads.
type GenericClientInput struct {
	Sequence   uint64
	Tick       uint64
	ObjectType string
	ObjectID   string
	TargetID   string
	Payload    []byte
}

// InputBufferMetrics contains cumulative runtime input-buffer counters.
type InputBufferMetrics struct {
	Buffered       uint64 `json:"buffered"`
	Consumed       uint64 `json:"consumed"`
	StaleRejected  uint64 `json:"staleRejected"`
	FutureRejected uint64 `json:"futureRejected"`
	Dropped        uint64 `json:"dropped"`
}

type inputBuffer struct {
	byClient map[string][]ClientInput
	metrics  InputBufferMetrics
	mu       sync.Mutex
}

func newInputBuffer() inputBuffer {
	return inputBuffer{byClient: map[string][]ClientInput{}}
}

func (buffer *inputBuffer) add(input ClientInput, currentTick uint64, config Config) error {
	input.ClientID = normalizeObjectKey(input.ClientID)
	if input.ClientID == "" {
		return ErrMissingClientInputID
	}
	if input.Tick == 0 {
		input.Tick = currentTick
	}
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = time.Now()
	}
	input.ObjectType = normalizeObjectKey(input.ObjectType)
	input.ObjectID = normalizeObjectKey(input.ObjectID)
	input.TargetID = normalizeObjectKey(input.TargetID)

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	if input.Tick < currentTick && currentTick-input.Tick > config.MaxInputPastTicks {
		buffer.metrics.StaleRejected++
		return ErrStaleClientInput
	}
	if input.Tick > currentTick+config.MaxInputFutureTicks {
		buffer.metrics.FutureRejected++
		return ErrFutureClientInput
	}

	inputs := append(buffer.byClient[input.ClientID], input)
	sortClientInputs(inputs)
	for len(inputs) > config.InputBufferLimit {
		inputs = inputs[1:]
		buffer.metrics.Dropped++
	}
	buffer.byClient[input.ClientID] = inputs
	buffer.metrics.Buffered++
	return nil
}

func (buffer *inputBuffer) consume(clientID string, tick uint64) []ClientInput {
	clientID = normalizeObjectKey(clientID)
	if clientID == "" {
		return nil
	}

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	inputs := buffer.byClient[clientID]
	if len(inputs) == 0 {
		return nil
	}

	consumed := make([]ClientInput, 0)
	remaining := inputs[:0]
	for _, input := range inputs {
		if input.Tick == tick {
			consumed = append(consumed, input)
			continue
		}
		remaining = append(remaining, input)
	}
	if len(remaining) == 0 {
		delete(buffer.byClient, clientID)
	} else {
		buffer.byClient[clientID] = remaining
	}
	buffer.metrics.Consumed += uint64(len(consumed))
	return consumed
}

func (buffer *inputBuffer) consumeForObject(clientID string, tick uint64, objectType, objectID string) []ClientInput {
	clientID = normalizeObjectKey(clientID)
	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)
	if clientID == "" {
		return nil
	}

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	inputs := buffer.byClient[clientID]
	if len(inputs) == 0 {
		return nil
	}

	matched := make([]ClientInput, 0)
	remaining := inputs[:0]
	for _, input := range inputs {
		targetTick := input.Tick == tick
		targetObject := (objectType == "" || strings.EqualFold(input.ObjectType, objectType)) &&
			(objectID == "" || input.ObjectID == objectID || input.TargetID == objectID)
		if targetTick && targetObject {
			matched = append(matched, input)
			continue
		}
		if !targetTick || !targetObject {
			remaining = append(remaining, input)
		}
	}
	if len(remaining) == 0 {
		delete(buffer.byClient, clientID)
	} else {
		buffer.byClient[clientID] = remaining
	}
	buffer.metrics.Consumed += uint64(len(matched))
	return matched
}

func (buffer *inputBuffer) metricsSnapshot() InputBufferMetrics {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	return buffer.metrics
}

func sortClientInputs(inputs []ClientInput) {
	sort.SliceStable(inputs, func(left, right int) bool {
		if inputs[left].Tick != inputs[right].Tick {
			return inputs[left].Tick < inputs[right].Tick
		}
		if inputs[left].Sequence != inputs[right].Sequence {
			return inputs[left].Sequence < inputs[right].Sequence
		}
		if inputs[left].ObjectType != inputs[right].ObjectType {
			return inputs[left].ObjectType < inputs[right].ObjectType
		}
		if inputs[left].ObjectID != inputs[right].ObjectID {
			return inputs[left].ObjectID < inputs[right].ObjectID
		}
		return inputs[left].TargetID < inputs[right].TargetID
	})
}
