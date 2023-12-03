package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"coscli/fsnotify"
	"coscli/util"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/tencentyun/cos-go-sdk-v5"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch Local Files And Sync them to cos objects",
	Long: `Watch local files

Format:
  ./coscli watch <local_path> <cos_destination_path> [flags]

Example:
  Sync New Files:
    ./coscli watch ~/ cos://examplebucket`,
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(2)(cmd, args); err != nil {
			return err
		}
		storageClass, _ := cmd.Flags().GetString("storage-class")
		if storageClass != "" && util.IsCosPath(args[0]) {
			logger.Fatalln("--storage-class can only use in upload")
			os.Exit(1)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		recursive, _ := cmd.Flags().GetBool("recursive")
		include, _ := cmd.Flags().GetString("include")
		exclude, _ := cmd.Flags().GetString("exclude")
		storageClass, _ := cmd.Flags().GetString("storage-class")
		rateLimiting, _ := cmd.Flags().GetFloat32("rate-limiting")
		partSize, _ := cmd.Flags().GetInt64("part-size")
		threadNum, _ := cmd.Flags().GetInt("thread-num")
		metaString, _ := cmd.Flags().GetString("meta")
		snapshotPath, _ := cmd.Flags().GetString("snapshot-path")
		meta, err := util.MetaStringToHeader(metaString)
		if err != nil {
			logger.Fatalln("Sync invalid meta, reason: " + err.Error())
		}
		// args[0]: 源地址
		// args[1]: 目标地址
		var snapshotDb *leveldb.DB
		if snapshotPath != "" {
			if snapshotDb, err = leveldb.OpenFile(snapshotPath, nil); err != nil {
				logger.Fatalln("Sync load snapshot error, reason: " + err.Error())
			}
			defer snapshotDb.Close()
		}

		if util.IsCosPath(args[0]) || !util.IsCosPath(args[1]) {
			logger.Fatalf("bad args local_path: %s, cos_destination_path: %s, See coscli watch --help", args[0], args[1])
			return
		}

		if !util.IsDirExists(args[0]) {
			logger.Fatalf("local_path: %s is not exists or not a dir ", args[0])
			return
		}

		op := &util.UploadOptions{
			StorageClass: storageClass,
			RateLimiting: rateLimiting,
			PartSize:     partSize,
			ThreadNum:    threadNum,
			Meta:         meta,
			SnapshotPath: snapshotPath,
			SnapshotDb:   snapshotDb,
		}

		watchAndUpload(args, recursive, include, exclude, op, snapshotPath)
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)

	watchCmd.Flags().BoolP("recursive", "r", false, "Synchronize objects recursively")
	watchCmd.Flags().String("include", "", "List files that meet the specified criteria")
	watchCmd.Flags().String("exclude", "", "Exclude files that meet the specified criteria")
	watchCmd.Flags().String("storage-class", "", "Specifying a storage class")
	watchCmd.Flags().Float32("rate-limiting", 0, "Upload or download speed limit(MB/s)")
	watchCmd.Flags().Int64("part-size", 32, "Specifies the block size(MB)")
	watchCmd.Flags().Int("thread-num", 5, "Specifies the number of concurrent upload or download threads")
	watchCmd.Flags().String("meta", "",
		"Set the meta information of the file, "+
			"the format is header:value#header:value, the example is Cache-Control:no-cache#Content-Encoding:gzip")
	watchCmd.Flags().String("snapshot-path", "", "This option is used to accelerate the incremental"+
		" upload of batch files or download objects in certain scenarios."+
		" If you use the option when upload files or download objects,"+
		" coscli will generate files to record the snapshot information in the specified directory."+
		" When the next time you upload files or download objects with the option, "+
		"coscli will read the snapshot information under the specified directory for incremental upload or incremental download. "+
		"The snapshot-path you specified must be a local file system directory can be written in, "+
		"if the directory does not exist, coscli creates the files for recording snapshot information, "+
		"else coscli will read snapshot information from the path for "+
		"incremental upload(coscli will only upload the files which haven't not been successfully uploaded to oss or"+
		" been locally modified) or incremental download(coscli will only download the objects which have not"+
		" been successfully downloaded or have been modified),"+
		" and update the snapshot information to the directory. "+
		"Note: The option record the lastModifiedTime of local files "+
		"which have been successfully uploaded in local file system or lastModifiedTime of objects which have been successfully"+
		" downloaded, and compare the lastModifiedTime of local files or objects in the next cp to decided whether to"+
		" skip the file or object. "+
		"In addition, coscli does not automatically delete snapshot-path snapshot information, "+
		"in order to avoid too much snapshot information, when the snapshot information is useless, "+
		"please clean up your own snapshot-path on your own immediately.")
}

type PeriodSynced struct {
	ChangedFiles map[string]int64
	mu           sync.Mutex
	Wg           sync.WaitGroup
	Quit         chan struct{}
}

func NewPeriodSynced() *PeriodSynced {
	return &PeriodSynced{
		ChangedFiles: make(map[string]int64),
		Quit:         make(chan struct{}),
	}
}

func (p *PeriodSynced) Update(filePath string, size int64) {
	p.mu.Lock()
	p.ChangedFiles[filePath] = size
	p.mu.Unlock()
}

func (p *PeriodSynced) UploadAndFlush(c *cos.Client, localPath, bucketName, cosPath string, op *util.UploadOptions) {
	swapMap := make(map[string]int64)

	// swap
	p.mu.Lock()
	backup := p.ChangedFiles
	p.ChangedFiles = swapMap
	swapMap = backup
	p.mu.Unlock()

	// upload
	p.Wg.Add(1)
	var i = 0
	for filePath, size := range swapMap {
		logger.Infof("sync (%d/%d) file: %s, %d Bytes", i, len(swapMap), filePath, size)
		i++
		UploadSingleFile(c, localPath, bucketName, cosPath, filePath, op)
	}
	p.Wg.Done()
}

func (p *PeriodSynced) Sync(c *cos.Client, localPath, bucketName, cosPath string, op *util.UploadOptions) {
	// mark start
	p.Wg.Add(1)

	for {
		select {
		case <-time.After(5 * time.Minute):
			p.UploadAndFlush(c, localPath, bucketName, cosPath, op)

		case <-p.Quit:
			p.UploadAndFlush(c, localPath, bucketName, cosPath, op)
			// mark end
			p.Wg.Done()
			return
		}
	}
}

func UploadSingleFile(c *cos.Client, localPath, bucketName, cosPath, filePath string, op *util.UploadOptions) {

	relPath, err := filepath.Rel(localPath, filePath)
	if err != nil {
		logger.Warnf("get relative path: %s ,err: %v", filePath, err)
		return
	}
	cosSyncPath := filepath.Join(cosPath, relPath)

	util.SyncSingleUpload(c, filePath, bucketName, cosSyncPath, op)
}

func watchAndUpload(args []string, recursive bool, include string, exclude string, op *util.UploadOptions,
	snapshotPath string) {

	_, localPath := util.ParsePath(args[0])
	bucketName, cosPath := util.ParsePath(args[1])

	c := util.NewClient(&config, &param, bucketName)

	addedDirs := make(map[string]bool)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Fatalf("fsnotify.NewWatcher, err:%v", err)
	}
	defer watcher.Close()

	Syncer := NewPeriodSynced()

	// 1. 递归添加文件夹到监控列表
	err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			logger.Warnf("watch sub path:%s, err: %s", path, err)
			return nil
		}

		mode := info.Mode()
		if mode.IsDir() {
			if !addedDirs[path] {
				err = watcher.Add(path)
				if err != nil {
					logger.Warnf("watch sub path:%s, err: %s", path, err)
					return err
				}
				addedDirs[path] = true
				logger.Warnf("watch sub path:%s ok", path)
			}
		} else if mode.IsRegular() {
			Syncer.Update(path, info.Size())
		} else {
			logger.Infof("skip file:%s, mode:%o", path, mode)
		}
		return nil
	})

	if err != nil {
		logger.Fatalf("filepath.Walk err: %v", err)
	}

	go Syncer.Sync(c, localPath, bucketName, cosPath, op)

	// 3. watch dir
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)
	signal.Notify(signalChan, syscall.SIGINT)
	signal.Notify(signalChan, syscall.SIGUSR1)
	changedFiles := make(map[string]bool)

loop:
	for {
		select {
		case event := <-watcher.Events:

			if strings.HasSuffix(event.Name, "coscli.log") {
				continue
			}

			// 如果是新创建的目录，将其添加到监视器
			if event.Op&fsnotify.Create == fsnotify.Create {
				fi, err := os.Stat(event.Name)
				if err == nil && fi.IsDir() {
					if !addedDirs[event.Name] {
						err := watcher.Add(event.Name)
						addedDirs[event.Name] = true
						logger.Infof("path: %s add to watch, err: %v", event.Name, err)
					}
				}
			}
			// 记录文件修改
			if event.Op&fsnotify.Write == fsnotify.Write {
				fi, err := os.Stat(event.Name)
				if err == nil && !fi.IsDir() {
					changedFiles[event.Name] = true
				}
			}

			// 如果是新创建的文件，将其添加到channel
			if event.Op&fsnotify.Close == fsnotify.Close {
				fi, err := os.Stat(event.Name)
				if err == nil && !fi.IsDir() {

					if !changedFiles[event.Name] {
						// logger.Infof("file: %s no changed closed,  skip sync to cos", event.Name)
					} else {
						logger.Infof("file: %s changed and closed, sync to cos", event.Name)
						Syncer.Update(event.Name, fi.Size())
						delete(changedFiles, event.Name)
					}
				}
			}
		case err := <-watcher.Errors:
			log.Println("监控错误:", err)

		case <-signalChan:
			fmt.Println("收到SIGTERM信号，正在关闭...")
			// 关闭watcher
			watcher.Close()

			// 处理修改 但是还没 closed文件
			for filePath := range changedFiles {
				if fi, err := os.Stat(filePath); err == nil {
					logger.Infof("file:%s change and not closed, put into chains to sync", filePath)
					Syncer.Update(filePath, fi.Size())
				}
			}

			// 触发事件处理并等待完成
			close(Syncer.Quit)
			Syncer.Wg.Wait()

			// 退出循环
			break loop
		}
	}
}
