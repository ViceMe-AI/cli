package replicacontent

const (
	MaxArchiveBytes        int64  = 100 << 20
	MaxArchiveEntries             = 20_000
	MaxFileCount                  = 10_000
	MaxFileBytes           uint64 = 100 << 20
	MaxExpandedBytes       uint64 = 500 << 20
	MaxCompressionRatio    uint64 = 100
	MaxArchivePathBytes           = 4_096
	MaxArchivePathDepth           = 128
	MaxArchiveSegmentBytes        = 255
)
