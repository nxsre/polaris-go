package sdk

import (
	"errors"
	"github.com/nxsre/polaris-go/log"
	model "github.com/polarismesh/polaris-go/pkg/model"
	specmodel "github.com/polarismesh/specification/source/go/api/v1/model"
	"sync"
)

const (
	NotExistedFileContent = string("@@not_existed@@")
)

type WatchFilesRequest struct {
	WatchFiles []WatchFile `json:"watch_files"`
}
type WatchFile struct {
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	FileName  string `json:"file_name"`
	Version   uint64 `json:"version"`

	// 旧版本内容
	content string `json:"-"`
}

func (w WatchFile) SetContent(str string) {
	w.content = str
}

type ConfigFilesWatcher struct {
	sdk        *SDK
	watchFiles map[string]WatchFile

	lock                sync.RWMutex
	changeListeners     []func(event model.ConfigFileChangeEvent)
	changeListenerChans []chan model.ConfigFileChangeEvent
}

// AddChangeListenerWithChannel 增加配置文件变更监听器
func (w *ConfigFilesWatcher) AddChangeListenerWithChannel() <-chan model.ConfigFileChangeEvent {
	w.lock.Lock()
	defer w.lock.Unlock()
	changeChan := make(chan model.ConfigFileChangeEvent, 64)
	w.changeListenerChans = append(w.changeListenerChans, changeChan)
	return changeChan
}

// AddChangeListener 增加配置文件变更监听器
func (w *ConfigFilesWatcher) AddChangeListener(cb model.OnConfigFileChange) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.changeListeners = append(w.changeListeners, cb)
	log.Infoln(w.changeListeners)
}

func (w *ConfigFilesWatcher) fireChangeEvent(event model.ConfigFileChangeEvent) {
	log.Infof("==== event: %+v %+v", w.changeListenerChans, w.changeListeners)
	for _, listenerChan := range w.changeListenerChans {
		log.Infof("++++ listenerChan event: %+v", event)
		listenerChan <- event
	}

	for _, changeListener := range w.changeListeners {
		log.Infof("@@@@ changeListener event: %+v", event)
		changeListener(event)
	}
}

var (
	num int64 = 0
)

func (w *ConfigFilesWatcher) Run() {
	for {
		select {
		case <-w.sdk.ctx.Done():
			return
		default:
			files := []WatchFile{}
			for _, file := range w.watchFiles {
				files = append(files, file)
			}
			resp, err := w.sdk.polarisClient.Resty().R().SetContext(w.sdk.ctx).SetBody(&WatchFilesRequest{files}).
				Post(PolarisUrl("/config/v1/WatchConfigFile"))
			if err != nil {
				log.Fatalln(err)
				return
			}

			configFileResp := ConfigFileResponse{}
			err = json.Unmarshal(resp.Body(), &configFileResp)
			if err != nil {
				log.Fatalln(err)
				return
			}

			// "/config/v1/WatchConfigFile" 接口在1分钟无更新时会返回 DataNoChange
			if configFileResp.GetCode() == specmodel.Code_DataNoChange {
				continue
			}

			// w.sdk.ctx Done
			if configFileResp.GetCode() != specmodel.Code_ExecuteSuccess {
				log.Errorln(configFileResp.GetCode())
				return
			}

			file := configFileResp.GetConfigFile()
			newfileResp, err := w.sdk.GetConfigFile(file.GetNamespace(), file.GetFileGroup(), file.GetFileName())

			oldContent := w.watchFiles[configFileResp.GetConfigFile().GetFileName()].content
			newContent := ""

			if newfileResp.GetCode() == specmodel.Code_ExecuteSuccess {
				newContent, err = newfileResp.GetConfigFile().GetContent()
				if err != nil {
					newContent = newfileResp.GetConfigFile().GetSourceContent()
				}
			}

			if newfileResp.GetCode() == specmodel.Code_NotFoundResource {
				newContent = NotExistedFileContent
			}

			log.Infof("[Config] update content. filename=%v, file = %+v, old content = %s, new content = %s",
				configFileResp.GetConfigFile().GetFileName(), configFileResp.GetConfigFile(), oldContent, newContent)

			var changeType model.ChangeType
			w.watchFiles[configFileResp.GetConfigFile().GetFileName()].SetContent(newContent)

			if oldContent == NotExistedFileContent && newContent != NotExistedFileContent {
				changeType = model.Added
				oldContent = ""
			} else if oldContent != NotExistedFileContent && newContent == NotExistedFileContent {
				changeType = model.Deleted
				// NotExistedFileContent 只用于内部删除标记，不应该透露给用户
				newContent = ""
			} else if oldContent != newContent {
				changeType = model.Modified
			} else {
				changeType = model.NotChanged
			}

			event := model.ConfigFileChangeEvent{
				ConfigFileMetadata: newfileResp.GetConfigFile(),
				OldValue:           oldContent,
				NewValue:           newContent,
				ChangeType:         changeType,
			}

			w.fireChangeEvent(event)
			//log.Infoln("新消息:", configFileResp.GetCode(), configFileResp.GetCode(), configFileResp.GetConfigFile(), resp.String())
			w.watchFiles[configFileResp.ConfigFile.GetFileName()] = WatchFile{
				FileName:  configFileResp.ConfigFile.GetFileName(),
				Namespace: configFileResp.ConfigFile.GetNamespace(),
				Group:     configFileResp.ConfigFile.GetFileGroup(),
				Version:   configFileResp.ConfigFile.GetVersion(),
			}
		}
	}
}

func (s *SDK) WatchConfigFiles(ns, group string, filenames ...string) (*ConfigFilesWatcher, error) {
	if len(filenames) == 0 {
		return nil, errors.New("least one file")
	}
	watchFiles := map[string]WatchFile{}
	for _, filename := range filenames {
		if filename == "" {
			log.Fatalln(filename)
		}
		configFile, err := s.GetConfigFile(ns, group, filename)
		if err != nil {
			return nil, err
		}
		if configFile.GetCode() != specmodel.Code_ExecuteSuccess {
			// 如果远程没有这个文件，设置 content 为 NotExistedFileContent
			if configFile.GetCode() == specmodel.Code_NotFoundResource {
				watchFiles[filename] = WatchFile{
					FileName:  filename,
					Group:     group,
					Namespace: ns,
					Version:   0,
					content:   NotExistedFileContent,
				}
			} else {
				log.Errorln(configFile.GetMessage())
			}
			continue
		}
		content, err := configFile.GetConfigFile().GetContent()
		if err != nil {
			log.Errorln(err)
			content = configFile.GetConfigFile().GetSourceContent()
		}
		watchFiles[filename] = WatchFile{
			FileName:  filename,
			Group:     group,
			Namespace: ns,
			Version:   configFile.GetConfigFile().GetVersion(),
			content:   content,
		}
	}

	watcher := &ConfigFilesWatcher{
		sdk:        s,
		watchFiles: watchFiles,
	}
	go watcher.Run()
	return watcher, nil
}
