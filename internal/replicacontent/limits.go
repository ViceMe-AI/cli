package replicacontent

const (
	MaxArchiveBytes     int64  = 100 << 20
	MaxFileCount               = 10_000
	MaxFileBytes        uint64 = 100 << 20
	MaxExpandedBytes    uint64 = 500 << 20
	MaxCompressionRatio uint64 = 100
)
