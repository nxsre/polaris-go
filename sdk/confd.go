package sdk

import (
	"context"
	"errors"
	jsoniter "github.com/json-iterator/go"
	"github.com/nxsre/polaris-go/log"
	"github.com/polarismesh/polaris-go/pkg/model"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Confd struct {
	prefix  string
	sdk     *SDK
	lock    sync.RWMutex
	watches map[string]*Watch
}

func (s *SDK) NewConfdClient() *Confd {
	return &Confd{
		sdk:     s,
		watches: map[string]*Watch{},
	}
}

func (c *Confd) GetValues(keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, k := range keys {
		namespace, group, fileName := parseKey(c.prefix, k)
		if !match(wildCardToRegexp("*"), fileName) {
			configFileResult, err := c.sdk.GetConfigFile(namespace, group, fileName)
			if err != nil {
				log.Errorln(err)
				continue
			}
			content, err := configFileResult.GetConfigFile().GetContent()
			if err != nil {
				log.Errorln(err)
				continue
			}
			result[filepath.Join(string(os.PathSeparator), group, fileName)] = content
		} else {
			pattern := regexp.MustCompilePOSIX(wildCardToRegexp(fileName))
			configFilesResult, err := c.sdk.GetConfigFileMetadataList(namespace, group)
			if err != nil {
				return nil, err
			}

			tmpResult := []map[string]interface{}{}
			for _, cfgFile := range configFilesResult.ConfigFileInfos {
				if pattern.MatchString(cfgFile.FileName) {
					var contents = map[string]interface{}{}
					configFile, err := c.sdk.GetConfigFile(namespace, group, cfgFile.FileName)
					if err != nil {
						log.Errorln(err)
						continue
					}

					fileConent, err := configFile.GetConfigFile().GetContent()
					if err != nil {
						log.Errorln(err)
						continue
					}
					if fileConent != "" {
						if err := jsoniter.UnmarshalFromString(fileConent, &contents); err != nil {
							log.Errorln(err)
							continue
						} else {
							tmpResult = append(tmpResult, contents)
						}
					}
				}
			}

			jb, err := jsoniter.MarshalIndent(&tmpResult, "", " ")
			result[filepath.Join(string(os.PathSeparator), group, fileName)] = string(jb)
		}
	}
	return result, nil
}

func (c *Confd) WatchPrefix(prefix string, keys []string, waitIndex uint64, stopChan chan bool) (uint64, error) {
	var err error
	// Create watch for each key
	watches := make(map[string]*Watch)
	c.lock.Lock()
	c.prefix = prefix
	for _, k := range keys {
		namespace, group, fileName := parseKey(prefix, k)

		watch, ok := c.watches[k]
		if !ok {
			watch, err = c.createWatch(namespace, group, fileName)
			if err != nil {
				c.lock.Unlock()
				return 0, err
			}
			c.watches[k] = watch
		}
		watches[k] = watch
	}
	c.lock.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancelRoutine := make(chan struct{})
	defer cancel()
	defer close(cancelRoutine)
	go func() {
		select {
		case <-stopChan:
			cancel()
		case <-cancelRoutine:
			return
		}
	}()

	notify := make(chan int64)
	// Wait for all watches
	for _, v := range watches {
		go v.WaitNext(ctx, c.sdk, int64(waitIndex), notify)
	}
	select {
	case nextRevision := <-notify:
		return uint64(nextRevision), err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Wait until revision is greater than lastRevision
func (w *Watch) WaitNext(ctx context.Context, c *SDK, lastRevision int64, notify chan<- int64) {
	//	如果是通配符模式，需要定时循环配置文件列表，配置文件增加时触发变更事件
	pattern := regexp.MustCompilePOSIX(wildCardToRegexp(w.filename))
	for {
		w.rwl.RLock()
		if w.revision > lastRevision {
			w.rwl.RUnlock()
			break
		}
		cond := w.cond
		w.rwl.RUnlock()

		if w.wildcard {
			log.Infoln("通配符匹配", w.filename)
			go func() {
				ticker := time.NewTicker(3 * time.Second)
				for {
					select {
					case <-ticker.C:
						configFilesResult, err := c.GetConfigFileMetadataList(w.namespace, w.group)
						if err != nil {
							log.Errorln(err)
							return
						}

						// 新增配置文件的逻辑
						needUpdate := false
						tmpConfigFiles := map[string]struct{}{}
						for _, configFile := range configFilesResult.ConfigFileInfos {
							if pattern.MatchString(configFile.FileName) {
								tmpConfigFiles[configFile.FileName] = struct{}{}
								// 当文件不存在监听列表，且已经发布过（ReleaseBy 不为空）时进行监听
								if _, ok := w.files[configFile.FileName]; !ok {
									log.Infoln("更新监听:", configFile.FileName)
									w.rwl.Lock()
									w.files[configFile.FileName] = struct{}{}
									w.rwl.Unlock()
									go watchFile(c, w.namespace, w.group, configFile.FileName, w)
									needUpdate = true
								}
							}
						}
						if needUpdate {
							return
						}

						// 删除配置文件
						for file, _ := range w.files {
							if _, ok := tmpConfigFiles[file]; !ok {
								needUpdate = true
							}
						}
						if needUpdate {
							w.update(w.revision + 1)
							return
						}

					case <-cond:
						return
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		select {
		case <-cond:
		case <-ctx.Done():
			return
		}
	}
	// We accept larger revision, so do not need to use RLock
	select {
	case notify <- w.revision:
	case <-ctx.Done():
	}
}

// Update revision
func (w *Watch) update(newRevision int64) {
	w.rwl.Lock()
	defer w.rwl.Unlock()
	w.revision = newRevision
	close(w.cond)
	w.cond = make(chan struct{})
}

// wildCardToRegexp converts a wildcard pattern to a regular expression pattern.
func wildCardToRegexp(pattern string) string {
	var result strings.Builder
	for i, literal := range strings.Split(pattern, "*") {
		// Replace * with .*
		if i > 0 {
			result.WriteString(".*")
		}
		// Quote any regular expression meta characters in the
		// literal text.
		result.WriteString(regexp.QuoteMeta(literal))
	}
	return result.String()
}

func match(pattern string, value string) bool {
	result, _ := regexp.MatchString(wildCardToRegexp(pattern), value)
	return result
}

func watchFile(client *SDK, namespace, group, fileName string, w *Watch) {
	cfgWatcher, err := client.WatchConfigFiles(namespace, group, fileName)
	if err != nil {
		log.Errorln(err)
		return
	}

	rch := cfgWatcher.AddChangeListenerWithChannel()
	log.Infof("Watch created on namespace:%s  group:%s  filename:%s", namespace, group, fileName)
	// 首次运行触发一次更新，然后等待配置中心
	w.update(w.revision + 1)
	for {
		log.Infof("检测配置变化 file:%s watcher: %p", fileName, w)
		select {
		case wresp := <-rch:
			log.Infof("更新事件: %+v", wresp)
			switch wresp.ChangeType {
			case model.Deleted:
				w.rwl.Lock()
				delete(w.files, fileName)
				w.rwl.Unlock()
				w.update(w.revision + 1)
				return
			case model.Added:
				w.update(w.revision + 1)
				log.Infoln("新增")
			case model.Modified:
				w.update(w.revision + 1)
				log.Infoln("变更")
			}
		}
	}
}

func (c *Confd) createWatch(namespace, group, fileName string) (*Watch, error) {
	w := &Watch{0, make(chan struct{}), sync.RWMutex{}, namespace, group, fileName, false, map[string]struct{}{}}

	if !match(wildCardToRegexp("*"), fileName) {
		go watchFile(c.sdk, namespace, group, fileName, w)
	} else {
		pattern := regexp.MustCompilePOSIX(wildCardToRegexp(fileName))
		w.wildcard = true
		configFilesResult, err := c.sdk.GetConfigFileMetadataList(namespace, group)
		if err != nil {
			return nil, err
		}
		for _, configFile := range configFilesResult.ConfigFileInfos {
			if pattern.MatchString(configFile.FileName) {
				if _, ok := w.files[configFile.FileName]; !ok {
					w.files[configFile.FileName] = struct{}{}
					go watchFile(c.sdk, namespace, group, configFile.FileName, w)
				}
			}
		}
	}

	return w, nil
}

// A watch only tells the latest revision
type Watch struct {
	// Last seen revision
	revision int64
	// A channel to wait, will be closed after revision changes
	cond chan struct{}
	// Use RWMutex to protect cond variable
	rwl sync.RWMutex

	namespace, group, filename string
	wildcard                   bool
	files                      map[string]struct{}
}

// 解析配置文件的 prefix 和 key，翻译为 namespace, group, filename 三个 Polaris 配置文件的概念
func parseKey(prefix, key string) (string, string, string) {
	if prefix == "" {
		arr := strings.SplitN(key, string(os.PathSeparator), 3)
		prefix = arr[1]
		key = arr[2]
	}

	path := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(key, prefix), string(os.PathSeparator)), string(os.PathSeparator), 2)
	namespace := filepath.Base(prefix)
	var group = "default"
	var fileName string

	if len(path) == 2 {
		group = path[0]
		fileName = path[1]
	} else {
		fileName = path[0]
	}

	if strings.TrimPrefix(namespace, string(os.PathSeparator)) == "" {
		namespace = "default"
	}

	if group == "" {
		group = "default"
	}

	return namespace, group, fileName
}

func GetInternalIP() (string, error) {
	conn, err := net.Dial("udp", "172.16.0.1:80")
	if err != nil {
		return "", errors.New("internal IP fetch failed, detail:" + err.Error())
	}
	defer conn.Close()

	// udp 面向无连接，所以这些东西只在你本地捣鼓
	res := conn.LocalAddr().String()
	res = strings.Split(res, ":")[0]
	return res, nil
}
