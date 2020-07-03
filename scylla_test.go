package gocql

import (
	"math"
	"runtime"
	"sync"
	"testing"
)

func TestScyllaConnPickerPickNilToken(t *testing.T) {
	t.Parallel()

	s := scyllaConnPicker{
		nrShards:  4,
		msbIgnore: 12,
	}

	t.Run("no conns", func(t *testing.T) {
		s.conns = []*Conn{{}}
		if s.Pick(token(nil)) != s.conns[0] {
			t.Fatal("expected connection")
		}
	})

	t.Run("one shard", func(t *testing.T) {
		s.conns = []*Conn{{}}
		if s.Pick(token(nil)) != s.conns[0] {
			t.Fatal("expected connection")
		}
	})

	t.Run("multiple shards", func(t *testing.T) {
		s.conns = []*Conn{nil, {}}
		if s.Pick(token(nil)) != s.conns[1] {
			t.Fatal("expected connection")
		}
		if s.Pick(token(nil)) != s.conns[1] {
			t.Fatal("expected connection")
		}
	})

	t.Run("multiple shards no conns", func(t *testing.T) {
		s.conns = []*Conn{nil, nil}
		if s.Pick(token(nil)) != nil {
			t.Fatal("expected nil")
		}
		if s.Pick(token(nil)) != nil {
			t.Fatal("expected nil")
		}
	})
}

func hammerConnPicker(t *testing.T, wg *sync.WaitGroup, s *scyllaConnPicker, loops int) {
	t.Helper()
	for i := 0; i < loops; i++ {
		if c := s.Pick(nil); c == nil {
			t.Error("unexpected nil")
		}
	}
	wg.Done()
}

func TestScyllaConnPickerHammerPickNilToken(t *testing.T) {
	t.Parallel()

	s := scyllaConnPicker{
		nrShards:  4,
		msbIgnore: 12,
	}
	s.conns = make([]*Conn, 100)
	for i := range s.conns {
		if i%7 == 0 {
			continue
		}
		s.conns[i] = &Conn{}
	}

	n := runtime.GOMAXPROCS(0)
	loops := 10000 / n

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go hammerConnPicker(t, &wg, &s, loops)
	}
	wg.Wait()
}

func TestScyllaConnPickerRemove(t *testing.T) {
	t.Parallel()

	s := scyllaConnPicker{
		nrShards:  4,
		msbIgnore: 12,
	}

	conn := mockConn(0)
	s.Put(conn)
	s.Put(mockConn(1))

	if s.nrConns != 2 {
		t.Error("added 2 connections, expected connection count to be 2")
	}

	s.Remove(conn)
	if s.nrConns != 1 {
		t.Errorf("removed 1 connection, expected connection count to be 1 but was %d", s.nrConns)
	}

	if s.conns[0] != nil {
		t.Errorf("Expected %v to be removed from it's position", conn)
	}
}

func mockConn(shard int) *Conn {
	return &Conn{
		scyllaSupported: scyllaSupported{
			shard:             shard,
			nrShards:          4,
			msbIgnore:         12,
			partitioner:       "org.apache.cassandra.dht.Murmur3Partitioner",
			shardingAlgorithm: "biased-token-round-robin",
		},
	}
}

func TestScyllaConnPickerShardOf(t *testing.T) {
	t.Parallel()

	s := scyllaConnPicker{
		nrShards:  4,
		msbIgnore: 12,
	}
	for _, test := range scyllaShardOfTests {
		if shard := s.shardOf(murmur3Token(test.token)); shard != test.shard {
			t.Errorf("wrong scylla shard calculated for token %d, expected %d, got %d", test.token, test.shard, shard)
		}
	}
}

func TestScyllaRandomConnPIcker(t *testing.T) {
	t.Parallel()

	t.Run("max iterations", func(t *testing.T) {
		s := &scyllaConnPicker{
			nrShards:  4,
			msbIgnore: 12,
			pos:       math.MaxUint64,
			conns:     []*Conn{nil, mockConn(1)},
		}

		if s.Pick(token(nil)) == nil {
			t.Fatal("expected connection")
		}
	})

	t.Run("async access of max iterations", func(t *testing.T) {
		s := &scyllaConnPicker{
			nrShards:  4,
			msbIgnore: 12,
			pos:       math.MaxUint64,
			conns:     []*Conn{nil, mockConn(1)},
		}

		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go pickLoop(t, s, 3, &wg)
		}
		wg.Wait()

		if s.pos != 8 {
			t.Fatalf("expected position to be 8 | actual %d", s.pos)
		}
	})
}

func pickLoop(t *testing.T, s *scyllaConnPicker, c int, wg *sync.WaitGroup) {
	t.Helper()
	for i := 0; i < c; i++ {
		if s.Pick(token(nil)) == nil {
			t.Fatal("expected connection")
		}
	}
	wg.Done()
}

func TestScyllaLWTExtParsing(t *testing.T) {
	t.Parallel()

	t.Run("init framer without cql extensions", func(t *testing.T) {
		t.Parallel()
		// mock connection without cql extensions, expected not to have
		// the `flagLWT` field being set in the framer created out of it
		conn := mockConn(0)
		f := newFramerWithExts(conn, conn, conn.compressor, conn.version, conn.cqlProtoExts)
		if f.flagLWT != 0 {
			t.Error("expected to have LWT flag uninitialized after framer init")
		}
	})

	t.Run("init framer with cql extensions", func(t *testing.T) {
		t.Parallel()
		// create a mock connection, add `lwt` cql protocol extension to it,
		// ensure that framer recognizes this extension and adjusts appropriately
		conn := mockConn(0)
		conn.cqlProtoExts = []cqlProtocolExtension{
			lwtAddMetadataMarkExt{
				lwtOptMetaBitMask: 1,
			},
		}
		framerWithLwtExt := newFramerWithExts(conn, conn, conn.compressor, conn.version, conn.cqlProtoExts)
		if framerWithLwtExt.flagLWT == 0 {
			t.Error("expected to have LWT flag to be set after framer init")
		}
	})
}
