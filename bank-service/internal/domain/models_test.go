package domain

import "testing"

func TestNewPage(t *testing.T) {
	page := NewPage([]int{1, 2}, 1, 2, 5)
	if page.TotalPages != 3 || page.TotalElements != 5 || page.Page != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestNewEmptyPageUsesJSONArray(t *testing.T) {
	page := NewPage[int](nil, 0, 20, 0)
	if page.Items == nil {
		t.Fatal("items must be an empty slice, not nil")
	}
}
