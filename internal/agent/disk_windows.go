//go:build windows

package agent

func diskSpace(path string) (int64, int64) { return 1 << 40, 2 << 40 }
