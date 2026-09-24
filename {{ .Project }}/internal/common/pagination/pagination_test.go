package pagination

import "testing"

func TestLimits(t *testing.T) {
	limits, err := New(0, 0)
	if err != nil || limits.MaxP != 10000 || limits.MaxS != 10000 {
		t.Fatalf("defaults: %v %v", limits, err)
	}
	for _, bad := range [][2]int{
		{-1, 10}, {10, -1}, {2147483648, 10}, {10, 2147483648},
	} {
		if _, err := New(bad[0], bad[1]); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	limits, err = New(3, 5)
	if err != nil {
		t.Fatal(err)
	}

	p, s, valid := limits.Normalize(nil, nil)
	if !valid || p != 1 || s != 1 {
		t.Fatalf("defaults: %d %d %v", p, s, valid)
	}
	page, size := int32(3), int32(5)
	if p, s, valid := limits.Normalize(&page, &size); !valid || p != 3 || s != 5 {
		t.Fatal(p, s, valid)
	}
	for _, value := range [][2]int32{
		{0, 1}, {1, 0}, {-1, 1}, {1, -1}, {4, 1}, {1, 6},
	} {
		p, s, valid := limits.Normalize(&value[0], &value[1])
		if valid || p != value[0] || s != value[1] {
			t.Fatalf("explicit values changed: %v -> %d %d", value, p, s)
		}
	}
}
