package sophon

import (
	"errors"
	"fmt"
)

// This file implements a minimal protobuf wire decoder for the sophon
// download manifest (SophonDownloadAssetsInfo) without requiring protobuf
// code generation. See the .proto schema:
//
//	message SophonDownloadAssetsInfo {
//	  repeated SophonDownloadAssetsInfoAsset assets = 1;
//	}
//	message SophonDownloadAssetsInfoAsset {
//	  string path = 1;
//	  repeated ...AssetChunk chunks = 2;
//	  uint32 type = 3;        // 0=file, 64=directory
//	  uint64 size = 4;
//	  string hash_md5 = 5;
//	}
//	message SophonDownloadAssetsInfoAssetChunk {
//	  string name = 1;
//	  string decompressed_hash_md5 = 2;
//	  uint64 offset = 3;
//	  uint64 compressed_size = 4;
//	  uint64 decompressed_size = 5;
//	  uint64 unspecified = 6;
//	  string compressed_hash_md5 = 7;
//	}

// Asset is one file in the game build.
type Asset struct {
	Path    string
	Type    uint32
	Size    int64
	HashMD5 string
	Chunks  []Chunk
}

// Chunk is one piece of an asset, written at Offset in the final file.
type Chunk struct {
	Name                string
	Offset              int64
	CompressedSize      int64
	CompressedHashMD5   string
	DecompressedSize    int64
	DecompressedHashMD5 string
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) eof() bool { return r.pos >= len(r.data) }

func (r *reader) varint() (uint64, error) {
	var v uint64
	var shift uint
	for {
		if r.pos >= len(r.data) {
			return 0, errors.New("varint: unexpected EOF")
		}
		b := r.data[r.pos]
		r.pos++
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return v, nil
		}
		shift += 7
		if shift > 63 {
			return 0, errors.New("varint: overflow")
		}
	}
}

func (r *reader) bytes() ([]byte, error) {
	n, err := r.varint()
	if err != nil {
		return nil, err
	}
	if uint64(r.pos)+n > uint64(len(r.data)) {
		return nil, errors.New("bytes: out of range")
	}
	b := r.data[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b, nil
}

// wireField reads a tag and returns (fieldNumber, wireType).
func (r *reader) wireField() (uint64, int, error) {
	tag, err := r.varint()
	if err != nil {
		return 0, 0, err
	}
	return tag >> 3, int(tag & 7), nil
}

// DecodeAssets parses a SophonDownloadAssetsInfo protobuf payload.
func DecodeAssets(data []byte) ([]Asset, error) {
	r := &reader{data: data}
	var assets []Asset
	for !r.eof() {
		field, wire, err := r.wireField()
		if err != nil {
			return nil, err
		}
		if field == 1 && wire == 2 {
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			a, err := decodeAsset(b)
			if err != nil {
				return nil, err
			}
			assets = append(assets, a)
			continue
		}
		if err := skipField(r, wire); err != nil {
			return nil, err
		}
	}
	return assets, nil
}

func decodeAsset(data []byte) (Asset, error) {
	r := &reader{data: data}
	var a Asset
	for !r.eof() {
		field, wire, err := r.wireField()
		if err != nil {
			return a, err
		}
		switch {
		case field == 1 && wire == 2:
			b, err := r.bytes()
			if err != nil {
				return a, err
			}
			a.Path = string(b)
		case field == 2 && wire == 2:
			b, err := r.bytes()
			if err != nil {
				return a, err
			}
			c, err := decodeChunk(b)
			if err != nil {
				return a, err
			}
			a.Chunks = append(a.Chunks, c)
		case field == 3 && wire == 0:
			v, err := r.varint()
			if err != nil {
				return a, err
			}
			a.Type = uint32(v)
		case field == 4 && wire == 0:
			v, err := r.varint()
			if err != nil {
				return a, err
			}
			a.Size = int64(v)
		case field == 5 && wire == 2:
			b, err := r.bytes()
			if err != nil {
				return a, err
			}
			a.HashMD5 = string(b)
		default:
			if err := skipField(r, wire); err != nil {
				return a, err
			}
		}
	}
	return a, nil
}

func decodeChunk(data []byte) (Chunk, error) {
	r := &reader{data: data}
	var c Chunk
	for !r.eof() {
		field, wire, err := r.wireField()
		if err != nil {
			return c, err
		}
		switch {
		case field == 1 && wire == 2:
			b, err := r.bytes()
			if err != nil {
				return c, err
			}
			c.Name = string(b)
		case field == 2 && wire == 2:
			b, err := r.bytes()
			if err != nil {
				return c, err
			}
			c.DecompressedHashMD5 = string(b)
		case field == 3 && wire == 0:
			v, err := r.varint()
			if err != nil {
				return c, err
			}
			c.Offset = int64(v)
		case field == 4 && wire == 0:
			v, err := r.varint()
			if err != nil {
				return c, err
			}
			c.CompressedSize = int64(v)
		case field == 5 && wire == 0:
			v, err := r.varint()
			if err != nil {
				return c, err
			}
			c.DecompressedSize = int64(v)
		case field == 7 && wire == 2:
			b, err := r.bytes()
			if err != nil {
				return c, err
			}
			c.CompressedHashMD5 = string(b)
		default:
			if err := skipField(r, wire); err != nil {
				return c, err
			}
		}
	}
	return c, nil
}

func skipField(r *reader, wire int) error {
	switch wire {
	case 0: // varint
		_, err := r.varint()
		return err
	case 1: // 64-bit
		if r.pos+8 > len(r.data) {
			return errors.New("skip: 64-bit out of range")
		}
		r.pos += 8
		return nil
	case 2: // length-delimited
		b, err := r.bytes()
		if err != nil {
			return err
		}
		_ = b
		return nil
	case 5: // 32-bit
		if r.pos+4 > len(r.data) {
			return errors.New("skip: 32-bit out of range")
		}
		r.pos += 4
		return nil
	}
	return fmt.Errorf("skip: unsupported wire type %d", wire)
}
