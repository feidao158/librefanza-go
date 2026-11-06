package catalog

import (
	"reflect"
	"testing"
)

func TestNormalizeProductID(t *testing.T) {
	tests := map[string]string{
		"abp123": "ABP-123", "abp00123dod": "ABP-123", "abp00001": "ABP-001",
		"104bshd16": "BSHD-16", "h_244sama083dod": "SAMA-083", "118ezd300_ar": "EZD-300",
		"55csct005tk2": "CSCT-005", "2APAA-101": "APAA-101", "td008dv1569d": "DV-1569",
		"DV1-662": "DV-1662", "7ADVO-9": "ADVO-009", "8ADV-F126": "ADVF-126",
		"d2cesd901": "CESD-901", "ut3fzr025": "FZR-025", "140c2344": "C-2344",
		"118docvr00001re01": "DOCVR-001", "h_1337wvr6d00004": "WVR6D-004",
		"abc": "abc", "123": "123", "123abc": "123abc", "abc123def": "ABC-123",
		"GHT-000A": "GHT-000", "13dsvr00003": "3DSVR-003", "h_093r1800294": "R18-294",
		"5520id00044": "20ID-044", "6id00044": "ID-044", "t28526": "T28-526",
		"55t015633": "T15-633", "T-99633": "T99-633", "4KDVAJ-594": "DVAJ-594",
		"DGCEAD-001": "CEAD-001", "DGCEMD-001": "CEMD-001", "DGCESD-001": "CESD-001",
		"DGCEDG-001": "DGCEDG-001", "FC2-PPV-4221749": "FC2-4221749",
		"FC2PPV-4221749": "FC2-4221749", "": "",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := NormalizeProductID(input); got != want {
				t.Fatalf("NormalizeProductID(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestProductIDVariations(t *testing.T) {
	got := ProductIDVariations("ABP-001")
	want := []string{"ABP-001", "ABP-0001", "abp001", "abp00001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("variations = %#v, want %#v", got, want)
	}
	if got := ProductIDVariations("XV-1001"); !reflect.DeepEqual(got, []string{"XV-1001", "xv1001", "xv01001"}) {
		t.Fatalf("duplicate variations were not removed: %#v", got)
	}
}

func TestProductIDValidityAndCompression(t *testing.T) {
	if IsProductID("ABC") || IsProductID("123") || !IsProductID("3DSVR-003") {
		t.Fatal("unexpected validity result")
	}
	if got := CompressProductID("abc00123"); got != "ABC-123" {
		t.Fatalf("compressed ID = %q", got)
	}
}
