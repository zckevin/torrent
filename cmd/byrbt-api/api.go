package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	// "strconv"
	"syscall"
	// "time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"

	"github.com/anacrolix/tagflag"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/fs"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	_ "net/http/pprof"
)

var (
	args = struct {
		DownloadDir string `help:"location to save torrent data"`
		MountDir    string `help:"location the torrent contents are made available"`
        Proxy string `help:socks5 proxy url, format like: "socks5://localhost:8080"`
        DownloadHost string

		DisableTrackers bool
		TestPeer        *net.TCPAddr
		ReadaheadBytes  tagflag.Bytes
		ListenAddr      *net.TCPAddr
	}{
		ReadaheadBytes: 1 << 27,
		ListenAddr:     &net.TCPAddr{},
    Proxy: "socks5://localhost:12345",
    DownloadHost: "https://withered-mouse-ac2c.byrvod.workers.dev/https/bt.byr.cn/",
	}
)

var client *torrent.Client

func exitSignalHandlers(fs *torrentfs.TorrentFS) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	for {
		<-c
		fs.Destroy()
		err := fuse.Unmount(args.MountDir)
		if err != nil {
			log.Print(err)
		}
	}
}

func RunTorrentClient() int {
	tagflag.Parse(&args)

	if args.MountDir == "" {
		os.Stderr.WriteString("y u no specify mountpoint?\n")
		return 2
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	conn, err := fuse.Mount(args.MountDir)
	if err != nil {
		log.Fatal(err)
	}
	defer fuse.Unmount(args.MountDir)
	// TODO: Think about the ramifications of exiting not due to a signal.
	defer conn.Close()

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = args.DownloadDir
	cfg.DisableTrackers = args.DisableTrackers
	cfg.NoUpload = true // Ensure that downloads are responsive.
	cfg.NoDHT = true
	// cfg.DisableIPv4 = true
	cfg.DefaultStorage = storage.NewBoltDB(args.DownloadDir)
	log.Println("after new boltdb")
	cfg.SetListenAddr(args.ListenAddr.String())

  cfg.ProxyURL = "socks5://localhost:12345"
	// cfg.ProxyURL = args.Proxy
	cfg.PeerID = "-TR2770-huyn91ga89sc"

	cfg.Debug = true

	client, err = torrent.NewClient(cfg)
	if err != nil {
		log.Print(err)
		return 3
	}

	fs := torrentfs.New(client)
	go exitSignalHandlers(fs)

	if err := fusefs.Serve(conn, fs); err != nil {
		log.Fatal(err)
	}
	<-conn.Ready
	if err := conn.MountError; err != nil {
		log.Fatal(err)
	}
	return 0
}

func CreateFileTask(name string, torrentFile io.Reader, fileIndex int) error {
	mi, err := metainfo.Load(torrentFile)
	if err != nil {
		return err
	}
	t, err := client.AddTorrent(mi)
	t.SetDisplayName(name)
	if err != nil {
		return err
	}
	for i, f := range t.Files() {
		log.Println(fileIndex, f.Path())
		if i == fileIndex {
			go func(f *torrent.File) {
			    // 50 MB prefetch
				r := f.NewReader2(4 * 1024 * 1024)
				io.Copy(ioutil.Discard, r)
			}(f)
		}
	}
	return nil
}

func RunApiServer() {
	router := gin.Default()

	// allow *
	router.Use(cors.Default())

	// Set a lower memory limit for multipart forms (default is 32 MiB)
	router.MaxMultipartMemory = 2 << 20 // 2 MiB

	router.GET("/", func(c *gin.Context) {
		client.WriteStatus(c.Writer)
	})

    router.GET("/list", func(c *gin.Context) {
        torrents := client.Torrents()
        var status []interface{}
        for _, t := range torrents {
            status = append(status, t.GetStatus())
        }
        c.JSON(200, status)
    })

	router.POST("/create", func(c *gin.Context) {
        id := c.PostForm("id")
        name := c.PostForm("name")
        resp, err := http.Get(args.DownloadHost + "download.php?id=" + id)
		if err != nil {
			log.Fatal(err)
		}
		err = CreateFileTask(name, resp.Body, 0)
		if err != nil {
		    log.Fatal(err)
		    c.String(http.StatusBadRequest, "fff")
		    return
		}
		c.String(http.StatusOK, id)

	    /*
		// Multipart form
		form, _ := c.MultipartForm()
		files := form.File["file"]
		indexStr := c.PostForm("fileIndex")
		fileIndex, err := strconv.Atoi(indexStr)
		if err != nil {
			log.Fatal(err)
		}

		for _, file := range files {
			log.Println(file.Filename)
			r, err := file.Open()
			if err != nil {
				log.Fatal(err)
			}
			CreateFileTask(r, fileIndex)
			// log.Println(r, fileIndex)
		}

		fmt.Println(c.PostForm("fileIndex"))
		c.String(http.StatusOK, fmt.Sprintf("%d files uploaded!", len(files)))
		*/
	})
	router.POST("/drop", func(c *gin.Context) {
		s := c.PostForm("infoHash")
		var h metainfo.Hash
		err := h.FromHexString(s)
		if err != nil {
			c.String(http.StatusOK, fmt.Sprintf("Infohash %s format error!", s))
			return
		}
		err = client.DropTorrent(h)
		if err != nil {
			c.String(http.StatusOK, fmt.Sprintf("Drop torrent %s error: %v", s, err))
			return
		}

		c.String(http.StatusOK, fmt.Sprintf("%s dropped!", s))
	})

	router.Run(":8062")
}

func main() {
  go func() {
      http.ListenAndServe("0.0.0.0:8081", nil)
  }()

	go RunApiServer()
	os.Exit(RunTorrentClient())
}
