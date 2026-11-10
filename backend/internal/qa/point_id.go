package qa

// Qdrant 点 ID 由「材料 + 序号 + 内容」确定性派生，不依赖 MySQL 自增切片主键。
// 至少一次投递下，同一任务被重复执行时写出的点集合完全一致，upsert 覆盖而非新增。
// 压到 2^53 以内可避免 JSON 大整数在其他客户端里丢精度。

import (
	"crypto/sha256"
	"hash/fnv"
	"strconv"
)

const maxPointID = uint64(1)<<53 - 1

// PointID 对同一份切片永远返回同一个 ID；正文或结构路径变化即变化。
func PointID(materialID int64, seq int, text, headingPath string) uint64 {
	digest := sha256.Sum256([]byte(text + "\x00" + headingPath))
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatInt(materialID, 10) + ":" + strconv.Itoa(seq) + ":"))
	_, _ = h.Write(digest[:16])
	return h.Sum64() & maxPointID
}
