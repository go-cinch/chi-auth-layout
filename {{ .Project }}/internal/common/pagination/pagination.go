package pagination

import "fmt"

// Limits apply to HTTP{{ if .Computed.enable_grpc_final }} and RPC{{ end }} calls. Zero configuration selects defaults.
type Limits struct {
	MaxP int32
	MaxS int32
}

func New(maxP, maxS int) (Limits, error) {
	if maxP == 0 {
		maxP = 10000
	}
	if maxS == 0 {
		maxS = 10000
	}
	if maxP < 1 || int64(maxP) > 2147483647 {
		return Limits{}, fmt.Errorf("pagination.maxP must be between 1 and 2147483647")
	}
	if maxS < 1 || int64(maxS) > 2147483647 {
		return Limits{}, fmt.Errorf("pagination.maxS must be between 1 and 2147483647")
	}
	return Limits{MaxP: int32(maxP), MaxS: int32(maxS)}, nil
}

func (l Limits) DefaultSize() int32 { return min(1, l.MaxS) }

// Normalize applies defaults only to omitted values. The boolean reports
// whether the requested page can contain data under these limits.
func (l Limits) Normalize(p, s *int32) (int32, int32, bool) {
	page, size := int32(1), l.DefaultSize()
	if p != nil {
		page = *p
	}
	if s != nil {
		size = *s
	}
	return page, size, page >= 1 && page <= l.MaxP && size >= 1 && size <= l.MaxS
}
