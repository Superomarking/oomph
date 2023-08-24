package player

import (
	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block"
	df_cube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/ethaniccc/float32-cube/cube"
	"github.com/oomph-ac/oomph/game"
	"github.com/oomph-ac/oomph/utils"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// GetChunkPos returns the chunk position from the given x and z coordinates.
func GetChunkPos(x, z int32) protocol.ChunkPos {
	return protocol.ChunkPos{x >> 4, z >> 4}
}

// ChunkExists returns true if the given chunk position was found in the map of chunks
func (p *Player) ChunkExists(pos protocol.ChunkPos) bool {
	_, ok := p.Chunk(pos)
	return ok
}

// AddChunk adds a chunk to the chunk map of the player. This function can also be used to replace existing chunks
func (p *Player) AddChunk(c *chunk.Chunk, pos protocol.ChunkPos) {
	p.chunks.Store(pos, c)
}

// Chunk returns a chunk from the given chunk position. If the chunk was found in the map, it will
// return the chunk and true.
func (p *Player) Chunk(pos protocol.ChunkPos) (*chunk.Chunk, bool) {
	v, ok := p.chunks.Load(pos)
	if !ok {
		return nil, false
	}

	return v.(*chunk.Chunk), true
}

// Block returns the block found at the given position
func (p *Player) Block(pos cube.Pos) world.Block {
	if pos.OutOfBounds(cube.Range(world.Overworld.Range())) {
		return block.Air{}
	}
	c, ok := p.Chunk(protocol.ChunkPos{int32(pos[0] >> 4), int32(pos[2] >> 4)})

	if !ok {
		return block.Air{}
	}
	rid := c.Block(uint8(pos[0]), int16(pos[1]), uint8(pos[2]), 0)

	b, _ := world.BlockByRuntimeID(rid)
	return b
}

// SetBlock sets a block at the given position to the given block
func (p *Player) SetBlock(pos cube.Pos, b world.Block) {
	if pos.OutOfBounds(cube.Range(world.Overworld.Range())) {
		return
	}

	rid := world.BlockRuntimeID(b)
	c, ok := p.Chunk(protocol.ChunkPos{int32(pos[0] >> 4), int32(pos[2] >> 4)})
	if !ok {
		return
	}

	c.SetBlock(uint8(pos[0]), int16(pos[1]), uint8(pos[2]), 0, rid)
}

// GetNearbyBBoxes returns a list of block bounding boxes that are within the given bounding box - which is usually
// the player's bounding box.
func (p *Player) GetNearbyBBoxes(aabb cube.BBox) []cube.BBox {
	grown := aabb.Grow(1e-4)
	min, max := grown.Min(), grown.Max()
	minX, minY, minZ := int(math32.Floor(min[0])), int(math32.Floor(min[1])), int(math32.Floor(min[2]))
	maxX, maxY, maxZ := int(math32.Ceil(max[0])), int(math32.Ceil(max[1])), int(math32.Ceil(max[2]))

	// A prediction of one BBox per block, plus an additional 2, in case
	var bboxList []cube.BBox
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
				pos := cube.Pos{x, y, z}
				boxes := p.Block(pos).Model().BBox(df_cube.Pos(pos), nil)
				for _, box := range boxes {
					b := game.DFBoxToCubeBox(box).Translate(pos.Vec3())

					if !b.IntersectsWith(aabb) {
						continue
					}

					bboxList = append(bboxList, b)
				}
			}
		}
	}
	return bboxList
}

// GetNearbyBlocks returns a list of blocks that are within the given bounding box.
func (p *Player) GetNearbyBlocks(aabb cube.BBox) map[cube.Pos]world.Block {
	grown := aabb.Grow(0.25)
	min, max := grown.Min(), grown.Max()
	minX, minY, minZ := int(math32.Floor(min[0])), int(math32.Floor(min[1])), int(math32.Floor(min[2]))
	maxX, maxY, maxZ := int(math32.Ceil(max[0])), int(math32.Ceil(max[1])), int(math32.Ceil(max[2]))

	// A prediction of one BBox per block, plus an additional 2, in case
	blocks := make(map[cube.Pos]world.Block)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
				pos := cube.Pos{x, y, z}
				blocks[pos] = p.Block(pos)
			}
		}
	}

	return blocks
}

// networkClientBreaksBlock is called when the client has data in PlayerAuthInput
// that is has broken a block.
func (p *Player) networkClientBreaksBlock(pos protocol.BlockPos) {
	// Get the position of the block the client is breaking
	spos := utils.BlockToCubePos(pos)
	b, _ := world.BlockByRuntimeID(air)

	p.SetBlock(spos, b)
}

// cleanChunks filters out any chunks that are out of the player's view, and returns a value of
// how many chunks were cleaned
func (p *Player) cleanChunks() {
	loc := p.mInfo.ServerPosition
	activePos := protocol.ChunkPos{int32(math32.Floor(loc[0])) >> 4, int32(math32.Floor(loc[2])) >> 4}

	// Unsubscribe from any chunks that are out of the player's view.
	p.chunks.Range(func(k, _ any) bool {
		pos := k.(protocol.ChunkPos)

		diffX, diffZ := pos[0]-activePos[0], pos[1]-activePos[1]
		dist := math32.Sqrt(float32(diffX*diffX) + float32(diffZ*diffZ))

		// If the distance is within the player's chunk view, leave it alone.
		if int32(dist) <= p.chunkRadius {
			return true
		}

		// The chunks are out of the player's view, so unsubscribe from them.
		p.chunks.Delete(pos)
		return true
	})
}

// clearAllChunks clears all chunks from the player's chunk map and unsubscribes from any cached chunks.
func (p *Player) clearAllChunks() {
	p.chunks.Range(func(k, _ any) bool {
		p.chunks.Delete(k)
		return true
	})
}
