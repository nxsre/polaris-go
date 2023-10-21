package main

import (
	"context"
	"encoding/json"
	"fmt"
	polaris "github.com/nxsre/polaris-go"
	"github.com/nxsre/polaris-go/api/configfiles"
	log "github.com/nxsre/polaris-go/log"
	"github.com/nxsre/polaris-go/sdk"
	"github.com/oklog/ulid/v2"
	"github.com/polarismesh/polaris-go/pkg/model"
	"time"
)

func main() {
	log.SetFormat("text")

	client, err := polaris.NewPolaris([]string{"http://polaris3.test.com:8090", "http://polaris2.test.com:8090", "http://polaris1.test.com:8090"}, "polaris", "polaris")
	if err != nil {
		log.Fatalln(err)
	}
	log.Infoln(client)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	_ = cancel
	ss := sdk.NewSDK(ctx, client)
	_, err = ss.GetConfigFile("default", "tcpwall", "blacklist-000.json")
	if err != nil {
		log.Fatalln(err)
	}

	// 获取配置文件组下的文件列表
	configFiles, err := ss.GetConfigFileMetadataList("default", "tcpwall")
	if err != nil {
		log.Fatalln(err)
	}
	_ = configFiles

	// 监听多个文件的事件
	files := []string{}
	for i := 0; i < 30; i++ {
		files = append(files, fmt.Sprintf("config-%d.json", i))
	}

	w, err := ss.WatchConfigFiles("default", "tcpwall", files...)

	// 方式一：添加监听器
	w.AddChangeListener(changeListener)

	// 方式二：添加监听器
	changeChan := w.AddChangeListenerWithChannel()

	log.Infoln("创建配置文件")
	// 创建并发布配置文件
	go func() {
		for i := 0; i < 30; i++ {
			ulidStr := ulid.Make().String()
			content, err := json.MarshalIndent(map[string]string{"aaaa": ulidStr}, "", "  ")
			if err != nil {
				return
			}
			_, err = configfiles.CreateAndPub(&configfiles.ConfigFile{
				// 每次传不同的 ReleaseName 才会自动发布，重复相同的 ReleaseName 配置文件的状态为 "编辑待发布"
				ReleaseName:        ulidStr,
				ReleaseDescription: "update blacklist",
				Namespace:          "default",
				Group:              "tcpwall",
				FileName:           fmt.Sprintf("config-%d.json", i),
				Content:            string(content),
				Comment:            "增加端口-xxxx",
				Format:             "json",
				Tags: []sdk.ConfigFileTag{
					{Key: "tagk", Value: ulidStr},
				},
			})
			time.Sleep(500 * time.Millisecond)
		}
	}()

	for {
		select {
		case event := <-changeChan:
			log.Infof("received change event by channel. %+v", event)
		}
	}
	//<-ctx.Done()
}

func changeListener(event model.ConfigFileChangeEvent) {
	log.Infof("received change event. %+v", event)
}
