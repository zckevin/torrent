package storage

import (
	"encoding/binary"
	"log"
	"path/filepath"
	"time"
	"bytes"

	"github.com/anacrolix/missinggo/expect"
	"github.com/boltdb/bolt"

	"github.com/anacrolix/torrent/metainfo"
)

const (
	// Chosen to match the usual chunk size in a torrent client. This way,
	// most chunk writes are to exactly one full item in bolt DB.
	chunkSize = 1 << 14
)

type boltDBClient struct {
	db *bolt.DB
}

type boltDBTorrent struct {
	cl *boltDBClient
	ih metainfo.Hash
}

func checkSizeLimit(db *bolt.DB) error {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(SequenceBucketKey)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Println("createbucketifnotexists()", err)
		return err
	}
	go func() {
		for {
			err := db.Update(func(tx *bolt.Tx) error {
				db := tx.Bucket(SequenceBucketKey)
				db2 := tx.Bucket(dataBucketKey)
				if db == nil || db2 == nil{
					return nil
				}
				stats := db2.Stats()
				log.Printf("%+v\n", stats)
				n := stats.LeafInuse
				// 100MB limit
				threshold := 5 * 1000 * 1000 * 1000
				if n > threshold {
					// toPrune := n - threshold
					toPrune := threshold >> 2
					deleted := 0
					cursor := db.Cursor()
					for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
						breakOut := false
						{
							cursor2 := db2.Cursor()
							prefix := v
							for k, _ := cursor2.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cursor2.Next() {
								_ = cursor2.Delete()
								deleted += chunkSize
								// log.Printf("delete %v \n", k)
								if deleted >= toPrune {
									breakOut = true
									break
								}
								_ = cursor.Delete()
							}
						}

						if breakOut {
							break
						}
					}
					log.Println("deleted: ", deleted)
				}
				return nil
			})
			if err != nil {
				log.Println("goroutine()", err)
			}
			time.Sleep(5 * time.Second)
		}
	}()
	return nil
}

func NewBoltDB(filePath string) ClientImpl {
	db, err := bolt.Open(filepath.Join(filePath, "bolt.db"), 0600, &bolt.Options{
		Timeout: time.Second,
	})
	expect.Nil(err)
	db.NoSync = true
	err = checkSizeLimit(db)
	if err != nil {
		log.Println(err)
	}
	return &boltDBClient{db}
}

func (me *boltDBClient) Close() error {
	return me.db.Close()
}

func (me *boltDBClient) OpenTorrent(info *metainfo.Info, infoHash metainfo.Hash) (TorrentImpl, error) {
	return &boltDBTorrent{me, infoHash}, nil
}

func (me *boltDBTorrent) Piece(p metainfo.Piece) PieceImpl {
	ret := &boltDBPiece{
		p:  p,
		db: me.cl.db,
		ih: me.ih,
	}
	copy(ret.key[:], me.ih[:])
	binary.BigEndian.PutUint32(ret.key[20:], uint32(p.Index()))
	return ret
}

func (boltDBTorrent) Close() error { return nil }
