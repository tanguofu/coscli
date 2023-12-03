package util

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	logger "github.com/sirupsen/logrus"
	"github.com/tencentyun/cos-go-sdk-v5"
)

const (
	CamUrl     = "http://metadata.tencentyun.com/meta-data/cam/security-credentials/"
	TIRoleName = "TIONE_QCSRole"
)

var (
	defaultAuthExpire    = time.Hour
	defaultCVMAuthExpire = int64(3600)
)

type TISecurityCredentials struct {
	TmpSecretId  string `json:"TmpSecretId"`
	TmpSecretKey string `json:"TmpSecretKey"`
	ExpiredTime  int64  `json:"ExpiredTime"`
	Expiration   string `json:"Expiration"`
	Token        string `json:"Token"`
	Code         string `json:"Code"`
}

var data TISecurityCredentials

func GetCamUrl(roleName string) string {

	if value, exists := os.LookupEnv("CAM_URL"); exists {
		return value
	}
	return CamUrl + roleName
}

func CamAuth(roleName string) TISecurityCredentials {

	if roleName == "" {
		logger.Fatalln("Get cam auth error : roleName not set")
		os.Exit(1)
	}

	// 创建HTTP客户端
	client := &http.Client{}

	// 创建一个5秒的超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", GetCamUrl(roleName), nil)
	if err != nil {
		logger.Fatalln("Get cam auth error : create request error", err)
		os.Exit(1)
	}
	req = req.WithContext(ctx)

	var res *http.Response

	// 创建一个HTTP GET请求并将上下文与其关联
	for i := 0; i < 5; i++ {
		response, err := client.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logger.Infof("Retrying in %d, Get TISecurityCredentials time out", i)
				time.Sleep(time.Second * 5)
				continue
			}

			logger.Warnf("Retrying in %d, Get TISecurityCredentials err:%v", i, err)
			time.Sleep(time.Second * 5)
			continue
		}
		res = response
		break
	}

	if res == nil {
		logger.Fatalln("Get cam auth error : create request error", err)
		os.Exit(1)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Fatalln("Get cam auth error : get response error", err)
		os.Exit(1)
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		logger.Fatalln("Get cam auth error : auth error")
		os.Exit(1)
	}

	logger.Infof("Get cam auth: %+v", data)

	if data.Code != "Success" {
		logger.Fatalln("Get cam auth error : response error", err)
		os.Exit(1)
	}

	return data
}

func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header, len(r.Header))
	for k, s := range r.Header {
		r2.Header[k] = append([]string(nil), s...)
	}
	return r2
}

type TICredentialTransport struct {
	RoleName     string
	Transport    http.RoundTripper
	secretID     string
	secretKey    string
	sessionToken string
	expiredTime  int64
	rwLocker     sync.RWMutex
}

// https://cloud.tencent.com/document/product/213/4934
func (t *TICredentialTransport) UpdateCredential(now int64) (string, string, string, error) {
	t.rwLocker.Lock()
	defer t.rwLocker.Unlock()
	if t.expiredTime > now+defaultCVMAuthExpire {
		return t.secretID, t.secretKey, t.sessionToken, nil
	}

	roleName := t.RoleName
	if roleName == "" {
		roleName = TIRoleName
	}

	urlname := GetCamUrl(roleName)

	resp, err := http.Get(urlname)
	if err != nil {
		return t.secretID, t.secretKey, t.sessionToken, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bs, _ := io.ReadAll(resp.Body)
		return t.secretID, t.secretKey, t.sessionToken, fmt.Errorf("call ti security-credentials failed, StatusCode: %v, Body: %v", resp.StatusCode, string(bs))
	}
	var cred TISecurityCredentials
	err = json.NewDecoder(resp.Body).Decode(&cred)
	if err != nil {
		return t.secretID, t.secretKey, t.sessionToken, err
	}
	if cred.Code != "Success" {
		return t.secretID, t.secretKey, t.sessionToken, fmt.Errorf("call ti security-credentials failed, Code:%v", cred.Code)
	}
	t.secretID, t.secretKey, t.sessionToken, t.expiredTime = cred.TmpSecretId, cred.TmpSecretKey, cred.Token, cred.ExpiredTime
	return t.secretID, t.secretKey, t.sessionToken, nil
}

func (t *TICredentialTransport) GetCredential() (string, string, string, error) {
	now := time.Now().Unix()
	t.rwLocker.RLock()
	// 提前 defaultCVMAuthExpire 获取重新获取临时密钥
	if t.expiredTime <= now+defaultCVMAuthExpire {
		expiredTime := t.expiredTime
		t.rwLocker.RUnlock()
		secretID, secretKey, secretToken, err := t.UpdateCredential(now)
		// 获取临时密钥失败但密钥未过期
		if err != nil && now < expiredTime {
			err = nil
		}
		return secretID, secretKey, secretToken, err
	}
	defer t.rwLocker.RUnlock()
	return t.secretID, t.secretKey, t.sessionToken, nil
}

func (t *TICredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ak, sk, token, err := t.GetCredential()
	if err != nil {
		return nil, err
	}
	req = cloneRequest(req)
	// 增加 Authorization header
	authTime := cos.NewAuthTime(defaultAuthExpire)
	cos.AddAuthorizationHeader(ak, sk, token, req, authTime)

	resp, err := t.transport().RoundTrip(req)
	return resp, err
}

func (t *TICredentialTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}
