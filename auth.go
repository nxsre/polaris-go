package polaris

// 基于 https://github.com/esiqveland/balancer 实现客户端负载均衡

import (
	"errors"
	"fmt"
	"github.com/esiqveland/balancer"
	"github.com/esiqveland/balancer/httpbalancer"
	"github.com/go-resty/resty/v2"
	"github.com/nxsre/polaris-go/log"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var (
	PolarisServer = "http://example.com"
	DefaultClient *Polaris
)

type Polaris struct {
	client *resty.Client
	logger *logrus.Logger
	token  string
}

func (p *Polaris) Resty() *resty.Client {
	return p.client
}

func NewPolaris(servers []string, username, password string) (*Polaris, error) {
	httpClient, err := BalancerClient(resty.New().GetClient(), servers)
	if err != nil {
		return nil, err
	}
	client := resty.NewWithClient(httpClient)
	client.SetLogger(log.With("app", "resty"))

	resp, err := client.SetRetryCount(3).SetRetryWaitTime(3*time.Second).R().
		EnableTrace().SetHeader("Content-Type", "application/json").
		SetBody(fmt.Sprintf(`{"name":"%s","password":"%s"}`, username, password)).
		Post("http://polaris.com/core/v1/user/login")

	var token string
	if err != nil {
		return nil, err
	} else {
		token = gjson.GetBytes(resp.Body(), "loginResponse.token").String()
	}

	polarisClient := &Polaris{
		client: client.SetRetryCount(3).SetRetryWaitTime(3*time.Second).SetHeader("X-Polaris-Token", token),
		token:  token,
	}

	if DefaultClient == nil {
		DefaultClient = polarisClient
	}
	return polarisClient, nil
}

func BalancerClient(client *http.Client, addrs []string) (*http.Client, error) {
	balance := WeightRoundRobinBalance{}
	for _, addr := range addrs {
		u, err := url.Parse(addr)
		if err != nil {
			log.Errorln(err)
			continue
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			log.Errorln(err)
			continue
		}
		if ip := net.ParseIP(u.Hostname()); ip == nil {
			ips, err := net.LookupIP(u.Hostname())
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				balance.Add(WeightHost{host: balancer.Host{Address: ip, Port: port}, weight: 1})
			}
		} else {
			balance.Add(WeightHost{host: balancer.Host{Address: ip, Port: port}, weight: 1})
		}

	}

	return httpbalancer.Wrap(client, &balance), nil
}

type WeightHost struct {
	host   balancer.Host
	weight int
}

type WeightRoundRobinBalance struct {
	curIndex int
	rss      []*WeightNode
	rsw      []int
}

type WeightNode struct {
	host            balancer.Host
	Weight          int //初始化时对节点约定的权重
	currentWeight   int //节点临时权重，每轮都会变化
	effectiveWeight int //有效权重, 默认与weight相同 , totalWeight = sum(effectiveWeight)  //出现故障就-1
}

//1, currentWeight = currentWeight + effectiveWeight
//2, 选中最大的currentWeight节点为选中节点
//3, currentWeight = currentWeight - totalWeight

func (r *WeightRoundRobinBalance) Add(host WeightHost) error {
	node := &WeightNode{
		host:   host.host,
		Weight: host.weight,
	}
	node.effectiveWeight = node.Weight
	r.rss = append(r.rss, node)
	return nil
}

func (r *WeightRoundRobinBalance) Next() (balancer.Host, error) {
	var best *WeightNode
	total := 0
	for i := 0; i < len(r.rss); i++ {
		w := r.rss[i]
		//1 计算所有有效权重
		total += w.effectiveWeight
		//2 修改当前节点临时权重
		w.currentWeight += w.effectiveWeight
		//3 有效权重默认与权重相同，通讯异常时-1, 通讯成功+1，直到恢复到weight大小
		if w.effectiveWeight < w.Weight {
			w.effectiveWeight++
		}

		//4 选中最大临时权重节点
		if best == nil || w.currentWeight > best.currentWeight {
			best = w
		}
	}

	if best == nil {
		return balancer.Host{}, errors.New("best is nil")
	}
	//5 变更临时权重为 临时权重-有效权重之和
	best.currentWeight -= total
	return best.host, nil
}

func (r *WeightRoundRobinBalance) Get() (balancer.Host, error) {
	return r.Next()
}

func (r *WeightRoundRobinBalance) Update() {
}
