package shardload

import (
	"fmt"
	"math/rand"
	"strconv"
)

var shardLoadFnMap = map[string]func(shardID string) float64{
	"constant": Constant,
	"shard-id": ShardID,
	"random":   Random,
}

func Fn(typ string) (func(shardID string) float64, error) {
	fn, ok := shardLoadFnMap[typ]
	if !ok {
		return nil, fmt.Errorf("unknown shard load func type: %s", typ)
	}

	return fn, nil
}

func Constant(_ string) float64 {
	return 1.0
}

func ShardID(shardID string) float64 {
	v, err := strconv.Atoi(shardID)
	if err != nil {
		return float64(len(shardID) + 1)
	}

	return float64(v)
}

func Random(_ string) float64 {
	return float64(rand.Intn(101))
}
