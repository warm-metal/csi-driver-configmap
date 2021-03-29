package cmmouter

import (
	"flag"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"testing"
)

func init() {
	testing.Init()
	klog.InitFlags(flag.CommandLine)
	flag.Set("logtostderr", "true")
	flag.Set("v", "7")
	flag.Parse()
}

func totalSizeOfCmData(data map[string]string) (size int) {
	for _, v := range data {
		size += len(v)
	}

	return
}

func TestOversizePolicyExactFullSize(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	volData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHead)
	if cmData["foo.txt"] != volData["foo.txt"] {
		t.Fail()
	}
}

func TestOversizePolicyTruncateHead(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	truncatedData := rand.String(configMapSizeHardLimit)
	volData := map[string]string{
		"foo.txt": "head-" + truncatedData,
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHead)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

func TestOversizePolicyTruncateHeadLineExactSize(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	truncatedData := "head-" + rand.String(configMapSizeHardLimit - len("head-"))
	volData := map[string]string{
		"foo.txt": "123\x0a" + truncatedData,
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHeadLine)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

func TestOversizePolicyTruncateHeadLineLessThanTheLimit(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	truncatedData := "head-" + rand.String(configMapSizeHardLimit - len("head-") - 3)
	volData := map[string]string{
		"foo.txt": "123\x0a" + truncatedData,
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHeadLine)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

func TestOversizePolicyTruncateHeadLineCase1(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": "0\n1\n2",
	}

	truncatedData := "onlyline"
	volData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit) + "\n" + truncatedData,
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHeadLine)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

func TestOversizePolicyTruncateTailLine(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	truncatedData := rand.String(configMapSizeHardLimit)
	volData := map[string]string{
		"foo.txt": truncatedData + "-tail",
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateTail)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

func TestOversizePolicyTruncateTailExactSize(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	truncatedData := rand.String(configMapSizeHardLimit - len("-tail\x0a")) + "-tail\x0a"
	volData := map[string]string{
		"foo.txt": truncatedData + "123",
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateTailLine)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

func TestOversizePolicyTruncateTailLineLessThanTheLimit(t *testing.T) {
	cmData := map[string]string{
		"foo.txt": rand.String(configMapSizeHardLimit),
	}

	truncatedData := rand.String(configMapSizeHardLimit - len("-tail\x0a") - len("123")) + "-tail\x0a"
	volData := map[string]string{
		"foo.txt": truncatedData + "1234",
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateTailLine)
	if cmData["foo.txt"] != truncatedData {
		t.Fail()
	}
}

// multiple files
func TestOversizePolicyExactFullSizeOnMultipleFiles(t *testing.T) {
	sizeDeltaFoo := 3
	sizeDeltaBar := 4
	size := configMapSizeHardLimit >> 1
	cmData := map[string]string{
		"foo.txt": rand.String(size - sizeDeltaFoo),
		"bar.txt": rand.String(size - sizeDeltaBar),
	}

	volData := map[string]string{
		"foo.txt": rand.String(size),
		"bar.txt": rand.String(size),
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHead)
	if cmData["foo.txt"] != volData["foo.txt"] {
		t.Log("foo.txt mismatched")
		t.Fail()
	}

	if cmData["bar.txt"] != volData["bar.txt"] {
		t.Log("bar.txt mismatched")
		t.Fail()
	}
}

func TestOversizePolicyTruncateHeadOfSecond(t *testing.T) {
	sizeDeltaFoo := 3
	sizeDeltaBar := 4
	size := configMapSizeHardLimit >> 1
	cmData := map[string]string{
		"foo.txt": rand.String(size - sizeDeltaFoo),
		"bar.txt": rand.String(size - sizeDeltaBar),
	}

	truncatedData := rand.String(size)
	volData := map[string]string{
		"foo.txt": rand.String(size),
		"bar.txt": "head-" + truncatedData,
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHead)
	if cmData["foo.txt"] != volData["foo.txt"] {
		t.Log("foo.txt mismatched")
		t.Fail()
	}

	if cmData["bar.txt"] != truncatedData {
		t.Log("bar.txt mismatched")
		t.Fail()
	}
}

func TestOversizePolicyTruncateHeadLineOfTheFirst(t *testing.T) {
	sizeDeltaFoo := 3
	sizeDeltaBar := 4
	size := configMapSizeHardLimit >> 1
	cmData := map[string]string{
		"foo.txt": rand.String(size - sizeDeltaFoo),
		"bar.txt": rand.String(size - sizeDeltaBar),
	}

	fooTxt := "78" + rand.String(size - sizeDeltaFoo)
	volData := map[string]string{
		"foo.txt": "12345\x0a" + fooTxt,
		"bar.txt": "1" + rand.String(size),
	}

	applyOversizePolicy(cmData, volData, totalSizeOfCmData(cmData), TruncateHeadLine)
	if cmData["foo.txt"] != fooTxt {
		t.Log("foo.txt mismatched")
		t.Fail()
	}

	if cmData["bar.txt"] != volData["bar.txt"] {
		t.Log("bar.txt mismatched")
		t.Fail()
	}
}
