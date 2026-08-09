package main

import (
	_ "embed"
	"kartio-kart/geo"
)

//go:embed assets/wario_stadium.col
var stadiumColBytes []byte

type Engine struct {
	bvh *geo.BVH
}

func NewEngine() (*Engine, error) {
	bvh := &geo.BVH{}
	err := bvh.UnmarshalBinary(stadiumColBytes)
	if err != nil {
		return nil, err
	}

	return &Engine{
		bvh: bvh,
	}, nil
}
