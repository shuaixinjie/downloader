package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

// Downloader 可设置并发数
type Downloader struct {
	concurrency int
}

func NewDownloader(concurrency int) *Downloader {
	return &Downloader{concurrency: concurrency}
}
// Download demo https://apache.claz.org/zookeeper/zookeeper-3.7.0/apache-zookeeper-3.7.0-bin.tar.gz
func (d *Downloader) Download(strURL, filename string) error {
	if filename == "" {
		filename = path.Base(strURL)  // apache-zookeeper-3.7.0-bin.tar.gz
	}
	resp, err := http.Head(strURL)  // resp.Header-->Accept-Ranges:[bytes]、Content-Length:[12387614]...
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK && resp.Header.Get("Accept-Ranges") == "bytes" {
		log.Println("sync download start...")
		return d.multiDownload(strURL, filename, int(resp.ContentLength))
	}
	log.Println("can't support sync download, normal download start...")
	return d.singleDownload(strURL, filename)
}

func (d *Downloader) multiDownload(strURL, filename string, contentLen int) error {
	partSize := contentLen / d.concurrency
	partDir := d.getPartDir(filename)
	os.Mkdir(partDir, 0777)  // 创建一个临时目录，结束的时候删了
	defer os.RemoveAll(partDir)

	var wg sync.WaitGroup
	wg.Add(d.concurrency)

	rangeStart := 0

	for i := 0; i < d.concurrency; i++ { // 开启多个协程并发下载
		go func(i, rangeStart int) {
			defer wg.Done()
			rangeEnd := rangeStart + partSize
			if i == d.concurrency - 1 {
				rangeEnd = contentLen
			}
			d.downloadPartial(strURL, filename, rangeStart, rangeEnd, i)
		}(i, rangeStart)
		rangeStart += partSize + 1
	}
	wg.Wait()
	d.merge(filename)
	return nil
}

func (d *Downloader) singleDownload(strURL, filename string) error {
	return nil
}

func (d *Downloader) downloadPartial(strURL, filename string, rangeStart, rangeEnd, i int) {
	if rangeStart >= rangeEnd {
		return
	}
	req, err := http.NewRequest("GET", strURL, nil) // GET
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd)) // 设置下载区间
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	partFile, err := os.OpenFile(d.getPartFilename(filename, i), flags, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer partFile.Close()
	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(partFile, resp.Body, buf) // copy resp.Body -> partFile
	if err != nil {
		if err == io.EOF {
			return
		}
		log.Fatal(err)
	}

}

// getPartDir 临时文件存放的目录
func (d *Downloader) getPartDir(filename string) string {
	return strings.SplitN(filename, ".", 2)[0] // apache-zookeeper-3.7.0-bin.tar.gz --> apache-zookeeper-3
}


// getPartFilename 构造部分文件的名字
func (d *Downloader) getPartFilename(filename string, partNum int) string {
	partDir := d.getPartDir(filename)
	return fmt.Sprintf("%s/%s-%d", partDir, filename, partNum)
}

func (d *Downloader) merge(filename string) error {
	destFile, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer destFile.Close()

	for i := 0; i < d.concurrency; i++ {
		partFileName := d.getPartFilename(filename, i)
		partFile, err := os.Open(partFileName)
		if err != nil {
			return err
		}
		io.Copy(destFile, partFile)  // 复制追加到主文件中
		partFile.Close()
		os.Remove(partFileName)
	}
	return nil
}