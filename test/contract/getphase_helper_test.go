//go:build contract

package contract_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/RVRTelecomunicaciones/sophia/pkg/contract"
)

// doGetPhase issues a direct GET against the phase URL using the
// stdlib client. It bypasses the cli's outbound port because the port
// doesn't (yet) expose a public GetPhase method — the cli's multiplexer
// reads phases through the change snapshot. The contract test asserts
// the wire endpoint exists and returns the canonical shape; once the
// cli adds GetPhase to the port, this helper can be deleted.
func doGetPhase(ctx context.Context, url string) (contract.PhaseResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return contract.PhaseResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return contract.PhaseResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return contract.PhaseResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.PhaseResponse{}, fmt.Errorf("getphase: status %d: %s", resp.StatusCode, body)
	}
	var out contract.PhaseResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return contract.PhaseResponse{}, err
	}
	return out, nil
}
