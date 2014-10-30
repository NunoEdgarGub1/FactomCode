package notaryapi

import (
	"bytes"
	"errors"
	"fmt"
	
	"encoding/binary"
	"sync"

)

type Chain struct {
	ChainID 	*Hash
	Name		string
	//Status	uint8
	
	
	Blocks 		[]*Block
	BlockMutex 	sync.Mutex	
	NextBlockID uint64	
}

type Block struct {

	//Marshalized
	Header *EBlockHeader
	EBEntries []*EBEntry

	//Not Marshalized
	Salt *Hash
	Chain *Chain	
	IsSealed bool
}

type EBInfo struct {

    EBHash *Hash
    FBHash *Hash
    FBBlockNum uint64
    ChainID *Hash
    //FBOffset uint64
    //EntryInfoArray *[]EntryInfo //not marshalized in db
    
}



func CreateBlock(chain *Chain, prev *Block, capacity uint) (b *Block, err error) {
	if prev == nil && chain.NextBlockID != 0 {
		return nil, errors.New("Previous block cannot be nil")
	} else if prev != nil && chain.NextBlockID == 0 {
		return nil, errors.New("Origin block cannot have a parent block")
	}
	
	b = new(Block)
	
	var prevHash *Hash
	if prev == nil {
		prevHash = EmptyHash()
	} else {
		prevHash, err = CreateHash(prev)
	}
	
	b.Header = NewEBlockHeader(chain.NextBlockID, prevHash, EmptyHash())
	
	b.Chain = chain
	
	b.EBEntries = make([]*EBEntry, 0, capacity)
	
	b.Salt = EmptyHash()
	
	b.IsSealed = false
	
	return b, err
}

func (b *Block) AddEBEntry(e *Entry) (err error) {
	h, err := CreateHash(e)
	if err != nil { return }
	
	s, err := CreateHash(b.Salt, h)
	if err != nil { return }


 	ebEntry := NewEBEntry(h, &b.Chain.ChainID.Bytes)	
 	ebEntry.SetIntTimeStamp(e.TimeStamp())
	
	b.EBEntries = append(b.EBEntries, ebEntry) 
	b.Salt = s
	
	return
}

func (b *Block) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer

	hashes := make([]*Hash, len(b.EBEntries))
	for i, entry := range b.EBEntries {
		data, _ := entry.MarshalBinary()
		hashes[i] = Sha(data)
		//fmt.Println("i=", i, ", hash=", hashes[i])
	}
	
	merkle := BuildMerkleTreeStore(hashes)
	b.Header.MerkleRoot = merkle[len(merkle) - 1]
	
	data, _ = b.Header.MarshalBinary()
	buf.Write(data)
	
	//binary.Write(&buf, binary.BigEndian, b.BlockID)
	//data, _ = b.PreviousHash.MarshalBinary()
	//buf.Write(data)
	
	count := uint64(len(b.EBEntries))
	binary.Write(&buf, binary.BigEndian, count)
	for i := uint64(0); i < count; i = i + 1 {
		data, _ := b.EBEntries[i].MarshalBinary()
		buf.Write(data)
	}
	
	data, _ = b.Salt.MarshalBinary()
	buf.Write(data)
	
	return buf.Bytes(), err
}

func (b *Block) MarshalledSize() uint64 {
	var size uint64 = 0
	
	//size += 8 // BlockID uint64
	//size += b.PreviousHash.MarshalledSize()
	
	size += b.Header.MarshalledSize()
	size += 8 // len(Entries) uint64
	size += b.Salt.MarshalledSize()
	
	for _, ebentry := range b.EBEntries {
		size += ebentry.MarshalledSize()
	}
	
	fmt.Println("block.MarshalledSize=", size)
	
	return size
}

func (b *Block) UnmarshalBinary(data []byte) (err error) {
	//b.BlockID, data = binary.BigEndian.Uint64(data[0:8]), data[8:]
	
	//b.PreviousHash = new(Hash)
	//b.PreviousHash.UnmarshalBinary(data)
	//data = data[b.PreviousHash.MarshalledSize():]
	
	ebh := new(EBlockHeader)
	ebh.UnmarshalBinary(data)
	b.Header = ebh
	
	data = data[ebh.MarshalledSize():]
	
	count, data := binary.BigEndian.Uint64(data[0:8]), data[8:]
	//fmt.Println("block.entry.count=", count)
	
	b.EBEntries = make([]*EBEntry, count)
	for i := uint64(0); i < count; i = i + 1 {
		b.EBEntries[i] = new(EBEntry)
		err = b.EBEntries[i].UnmarshalBinary(data)
		if err != nil { return }
		data = data[b.EBEntries[i].MarshalledSize():]
	}
	
	b.Salt = new(Hash)
	b.Salt.UnmarshalBinary(data)
	data = data[b.Salt.MarshalledSize():]
	
	return nil
}


func (b *EBInfo) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer

	data, _ = b.EBHash.MarshalBinary()
	buf.Write(data)
	
	data, _ = b.FBHash.MarshalBinary()
	buf.Write(data)
	
	binary.Write(&buf, binary.BigEndian, b.FBBlockNum)
	
	data, _ = b.ChainID.MarshalBinary()	
	buf.Write(data) 
	
	return buf.Bytes(), err
}

func (b *EBInfo) MarshalledSize() uint64 {
	var size uint64 = 0
	size += 33	//b.EBHash
	size += 33  //b.FBHash
	size += 8 	//b.FBBlockNum
	size += 33 	//b.ChainID	
	
	return size
}

func (b *EBInfo) UnmarshalBinary(data []byte) (err error) {
	b.EBHash = new(Hash)
	b.EBHash.UnmarshalBinary(data[:33])

	data = data[33:]
	b.FBHash = new(Hash)
	b.FBHash.UnmarshalBinary(data[:33])	
	
	data = data[33:]
	b.FBBlockNum = binary.BigEndian.Uint64(data[0:8])
	
	data = data[8:]
	b.ChainID = new(Hash)
	b.ChainID.UnmarshalBinary(data[:33])	
	
	return nil
}

func (b *Chain) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer

	data, _ = b.ChainID.MarshalBinary()
	buf.Write(data)
	
	data = []byte (b.Name)
	count := len(data)
	
	binary.Write(&buf, binary.BigEndian, uint64(count))	

	buf.Write(data)
	
	return buf.Bytes(), err
}

func (b *Chain) MarshalledSize() uint64 {
	var size uint64 = 0
	size += 33	//b.ChainID
	size += 1  // string length
	size += uint64(len(b.Name)) 	
	
	return size
}

func (b *Chain) UnmarshalBinary(data []byte) (err error) {
	b.ChainID = new(Hash)
	b.ChainID.UnmarshalBinary(data[:33])

	data = data[33:]
	count := binary.BigEndian.Uint64(data[0:8])
	
	data = data[8:]
	b.Name = string(data[:count])
	
	return nil
}

func (b *Chain) GenerateIDFromName(name string) (chainID *Hash, err error) {
	b.ChainID = Sha([]byte(name))
	
	return b.ChainID, nil
}