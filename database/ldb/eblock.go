package ldb

import (
	"encoding/binary"
	"errors"
	"github.com/FactomProject/FactomCode/common"
	"github.com/FactomProject/goleveldb/leveldb"
	"log"

	"github.com/FactomProject/goleveldb/leveldb/util"
)

// FetchEBEntriesFromQueue gets all of the ebentries that have not been processed
/*func (db *LevelDb) FetchEBEntriesFromQueue(chainID *[]byte, startTime *[]byte) (ebentries []*common.EBEntry, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var fromkey []byte = []byte{byte(TBL_ENTRY_QUEUE)} // Table Name (1 bytes)
	fromkey = append(fromkey, *chainID...)             // Chain Type (32 bytes)
	fromkey = append(fromkey, *startTime...)           // Timestamp  (8 bytes)

	var tokey []byte = []byte{byte(TBL_ENTRY_QUEUE)} // Table Name (4 bytes)
	tokey = append(tokey, *chainID...)               // Chain Type (4 bytes)

	binaryTimestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(binaryTimestamp, uint64(time.Now().Unix()))
	tokey = append(tokey, binaryTimestamp...) // Timestamp (8 bytes)

	ebEntrySlice := make([]*common.EBEntry, 0, 10)

	iter := db.lDb.NewIterator(&util.Range{Start: fromkey, Limit: tokey}, db.ro)

	for iter.Next() {
		if bytes.Equal(iter.Value(), []byte{byte(STATUS_IN_QUEUE)}) {
			key := make([]byte, len(iter.Key()))
			copy(key, iter.Key())
			ebEntry := new(common.EBEntry)

			ebEntry.SetTimeStamp(key[33:41])
			ebEntry.SetHash(key[41:73])
			ebEntrySlice = append(ebEntrySlice, ebEntry)
		}

	}
	iter.Release()
	err = iter.Error()

	return ebEntrySlice, nil
}
*/
// ProcessEBlockBatche inserts the EBlock and update all it's ebentries in DB
func (db *LevelDb) ProcessEBlockBatch(eblock *common.EBlock) error {

	if eblock != nil {
		if db.lbatch == nil {
			db.lbatch = new(leveldb.Batch)
		}

		defer db.lbatch.Reset()

		if len(eblock.EBEntries) < 1 {
			return errors.New("Empty eblock!")
		}

		binaryEblock, err := eblock.MarshalBinary()
		if err != nil {
			return err
		}

		// Insert the binary entry block
		var key []byte = []byte{byte(TBL_EB)}
		key = append(key, eblock.EBHash.Bytes...)
		db.lbatch.Put(key, binaryEblock)

		// Insert the entry block merkle root cross reference
		key = []byte{byte(TBL_EB_MR)}
		key = append(key, eblock.MerkleRoot.Bytes...)
		binaryEBHash, _ := eblock.EBHash.MarshalBinary()
		db.lbatch.Put(key, binaryEBHash)

		// Insert the entry block number cross reference
		key = []byte{byte(TBL_EB_CHAIN_NUM)}
		key = append(key, eblock.Chain.ChainID.Bytes...)
		bytes := make([]byte, 8)
		binary.BigEndian.PutUint64(bytes, eblock.Header.BlockID)
		key = append(key, bytes...)
		db.lbatch.Put(key, binaryEBHash)

		// Update entry process queue for each entry in eblock
		for i := 0; i < len(eblock.EBEntries); i++ {
			var ebEntry common.EBEntry = *eblock.EBEntries[i]

			if isLookupDB {
				// Create an EntryInfo and insert it into db
				var entryInfo = new(common.EntryInfo)
				entryInfo.EntryHash = ebEntry.Hash()
				entryInfo.EBHash = eblock.EBHash
				entryInfo.EBBlockNum = eblock.Header.BlockID
				var entryInfoKey []byte = []byte{byte(TBL_ENTRY_INFO)}
				entryInfoKey = append(entryInfoKey, entryInfo.EntryHash.Bytes...)
				binaryEntryInfo, _ := entryInfo.MarshalBinary()
				db.lbatch.Put(entryInfoKey, binaryEntryInfo)
			}

		}

		err = db.lDb.Write(db.lbatch, db.wo)
		if err != nil {
			log.Println("batch failed %v\n", err)
			return err
		}

	}
	return nil
}

// FetchEBInfoByHash gets an EBInfo obj
func (db *LevelDb) FetchEBInfoByHash(ebHash *common.Hash) (ebInfo *common.EBInfo, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var key []byte = []byte{byte(TBL_EB_INFO)}
	key = append(key, ebHash.Bytes...)
	data, err := db.lDb.Get(key, db.ro)

	if data != nil {
		ebInfo = new(common.EBInfo)
		ebInfo.UnmarshalBinary(data)
	}

	return ebInfo, nil
}

// FetchEBlockByMR gets an entry block by merkle root from the database.
func (db *LevelDb) FetchEBlockByMR(eBMR *common.Hash) (eBlock *common.EBlock, err error) {
	eBlockHash, _ := db.FetchEBHashByMR(eBMR)

	if eBlockHash != nil {
		eBlock, _ = db.FetchEBlockByHash(eBlockHash)
	}

	return eBlock, nil
}

// FetchEntryBlock gets an entry by hash from the database.
func (db *LevelDb) FetchEBlockByHash(eBlockHash *common.Hash) (eBlock *common.EBlock, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var key []byte = []byte{byte(TBL_EB)}
	key = append(key, eBlockHash.Bytes...)
	data, err := db.lDb.Get(key, db.ro)

	if data != nil {
		eBlock = new(common.EBlock)
		eBlock.UnmarshalBinary(data)
	}
	return eBlock, nil
}

// FetchEBHashByMR gets an entry by hash from the database.
func (db *LevelDb) FetchEBHashByMR(eBMR *common.Hash) (eBlockHash *common.Hash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var key []byte = []byte{byte(TBL_EB_MR)}
	key = append(key, eBMR.Bytes...)
	data, err := db.lDb.Get(key, db.ro)

	if data != nil {
		log.Println("data:%v", data)
		eBlockHash = new(common.Hash)
		eBlockHash.UnmarshalBinary(data)
		log.Println("eBlockHash:%v", eBlockHash.Bytes)
	}
	return eBlockHash, nil
}

// InsertChain inserts the newly created chain into db
func (db *LevelDb) InsertChain(chain *common.EChain) (err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	if db.lbatch == nil {
		db.lbatch = new(leveldb.Batch)
	}
	defer db.lbatch.Reset()

	binaryChain, _ := chain.MarshalBinary()

	var chainByHashKey []byte = []byte{byte(TBL_CHAIN_HASH)}
	chainByHashKey = append(chainByHashKey, chain.ChainID.Bytes...)

	db.lbatch.Put(chainByHashKey, binaryChain)

	var chainByNameKey []byte = []byte{byte(TBL_CHAIN_NAME)}
	chainByNameKey = append(chainByNameKey, []byte(common.EncodeChainNameToString(chain.Name))...)

	// Cross reference to chain id
	db.lbatch.Put(chainByNameKey, chain.ChainID.Bytes)

	err = db.lDb.Write(db.lbatch, db.wo)
	if err != nil {
		log.Println("batch failed %v\n", err)
		return err
	}

	return nil
}

// FetchChainByHash gets a chain by chainID
func (db *LevelDb) FetchChainByHash(chainID *common.Hash) (chain *common.EChain, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var key []byte = []byte{byte(TBL_CHAIN_HASH)}
	key = append(key, chainID.Bytes...)
	data, err := db.lDb.Get(key, db.ro)

	if data != nil {
		chain = new(common.EChain)
		chain.UnmarshalBinary(data)
	}
	return chain, nil
}

// FetchChainIDByName gets a chainID by chain name
func (db *LevelDb) FetchChainIDByName(chainName [][]byte) (chainID *common.Hash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	name := common.EncodeChainNameToString(chainName)

	var key []byte = []byte{byte(TBL_CHAIN_NAME)}
	key = append(key, []byte(name)...)
	data, err := db.lDb.Get(key, db.ro)

	if data != nil {
		chainID = new(common.Hash)
		chainID.Bytes = make([]byte, len(data))
		copy(chainID.Bytes, data)
	}
	return chainID, nil
}

// FetchChainByName gets a chain by chain name
func (db *LevelDb) FetchChainByName(chainName [][]byte) (chain *common.EChain, err error) {

	chainID, _ := db.FetchChainIDByName(chainName)

	if chainID != nil {
		chain, _ = db.FetchChainByHash(chainID)
	}
	return chain, nil

}

// FetchAllChainByName gets all of the chains under the path - name
func (db *LevelDb) FetchAllChainsByName(chainName [][]byte) (chains *[]common.EChain, err error) {

	chainSlice := make([]common.EChain, 0, 10)

	chainIDSlice, _ := db.FetchAllChainIDsByName(chainName)

	for _, chainID := range *chainIDSlice {
		chain, _ := db.FetchChainByHash(&chainID)
		if chain != nil {
			chainSlice = append(chainSlice, *chain)
		}
	}

	return &chainSlice, nil
}

// FetchAllChainIDsByName gets all of the chainIDs under the path - name
func (db *LevelDb) FetchAllChainIDsByName(chainName [][]byte) (chainIDs *[]common.Hash, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	name := common.EncodeChainNameToString(chainName)

	var fromkey []byte = []byte{byte(TBL_CHAIN_NAME)} // Table Name (1 bytes)
	fromkey = append(fromkey, []byte(name)...)        // Chain Type (32 bytes)
	var tokey []byte = addOneToByteArray(fromkey)

	chainIDSlice := make([]common.Hash, 0, 10)

	iter := db.lDb.NewIterator(&util.Range{Start: fromkey, Limit: tokey}, db.ro)

	for iter.Next() {
		chainID := new(common.Hash)
		chainID.Bytes = make([]byte, len(iter.Value()))
		copy(chainID.Bytes, iter.Value())
		chainIDSlice = append(chainIDSlice, *chainID)
	}
	iter.Release()
	err = iter.Error()

	return &chainIDSlice, nil
}

// FetchAllEBlocksByChain gets all of the blocks by chain id
func (db *LevelDb) FetchAllEBlocksByChain(chainID *common.Hash) (eBlocks *[]common.EBlock, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var fromkey []byte = []byte{byte(TBL_EB_CHAIN_NUM)} // Table Name (1 bytes)
	fromkey = append(fromkey, []byte(chainID.Bytes)...) // Chain Type (32 bytes)
	var tokey []byte = addOneToByteArray(fromkey)

	eBlockSlice := make([]common.EBlock, 0, 10)

	iter := db.lDb.NewIterator(&util.Range{Start: fromkey, Limit: tokey}, db.ro)

	for iter.Next() {
		eBlockHash := new(common.Hash)
		eBlockHash.UnmarshalBinary(iter.Value())

		var key []byte = []byte{byte(TBL_EB)}
		key = append(key, eBlockHash.Bytes...)
		data, _ := db.lDb.Get(key, db.ro)

		if data != nil {
			eBlock := new(common.EBlock)
			eBlock.UnmarshalBinary(data)
			eBlock.EBHash = eBlockHash
			eBlockSlice = append(eBlockSlice, *eBlock)
		}
	}
	iter.Release()
	err = iter.Error()

	return &eBlockSlice, nil
}

// FetchAllEBInfosByChain gets all of the entry block infos by chain id
func (db *LevelDb) FetchAllEBInfosByChain(chainID *common.Hash) (eBInfos *[]common.EBInfo, err error) {
	db.dbLock.Lock()
	defer db.dbLock.Unlock()

	var fromkey []byte = []byte{byte(TBL_EB_CHAIN_NUM)} // Table Name (1 bytes)
	fromkey = append(fromkey, []byte(chainID.Bytes)...) // Chain Type (32 bytes)
	var tokey []byte = addOneToByteArray(fromkey)

	eBInfoSlice := make([]common.EBInfo, 0, 10)

	iter := db.lDb.NewIterator(&util.Range{Start: fromkey, Limit: tokey}, db.ro)

	for iter.Next() {
		eBlockHash := new(common.Hash)
		eBlockHash.Bytes = make([]byte, len(iter.Value()))
		copy(eBlockHash.Bytes, iter.Value())

		var key []byte = []byte{byte(TBL_EB_INFO)}
		key = append(key, eBlockHash.Bytes...)
		data, _ := db.lDb.Get(key, db.ro)

		if data != nil {
			eBInfo := new(common.EBInfo)
			eBInfo.UnmarshalBinary(data)
			eBInfoSlice = append(eBInfoSlice, *eBInfo)
		}
	}
	iter.Release()
	err = iter.Error()

	return &eBInfoSlice, nil
}

// Internal db use only
func addOneToByteArray(input []byte) (output []byte) {
	if input == nil {
		return []byte{byte(1)}
	}
	output = make([]byte, len(input))
	copy(output, input)
	for i := len(input); i > 0; i-- {
		if output[i-1] <= 255 {
			output[i-1] = output[i-1] + 1
			break
		}
	}
	return output
}
