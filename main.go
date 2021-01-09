package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	author        = "starx"
	apiCourseList = "http://www.fjpit.com/studentportal.php?m=Wapxkcz&a=yjxklb"
	apiPostCourse = "http://www.fjpit.com/studentportal.php?m=Wapxkcz&a=yjxkbc"
	//
	get  		  = "GET"
	post 		  = "POST"
	empty = ""
	//
	httpOK        = 200
	//
	all       = "all"
	full      = "已满"
	errorCode = 410
	available = "未满"
	//
	retryMAX = 10
)

var (
	token        = ""
	cookieJar, _ = cookiejar.New(nil)
	payload      io.Reader
	wg           sync.WaitGroup
	retry        = true
	sleep        = 10
)

type DATAs struct {
	Data DATA 		`json:"data"`
}

type DATA struct {
	Wrapper []COURSE `json:"data"`
}

type COURSE struct {
	Id      string `json:"id"`
	Block   string `json:"kkxqmc"`
	Point   string `json:"kcxf"`
	Name    string `json:"kcmc"`
	Teacher string `json:"zdjsxm"`
	Class   string `json:"bjmc"`
	Max     string `json:"xkrsrl"`
	Ordered string `json:"xkyxrs"`
}

type REPLY struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
}


func check(stop bool,errs ...error)  {
	for _,e := range errs{
		if e != nil{
			if stop {
				panic(e)
			}else {
				fmt.Println(e.Error())
			}
		}
	}
}

func newRequest(method string,destUrl string,body io.Reader) *http.Request {
	req,err := http.NewRequest(method,destUrl,body)
	check(true,err)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Linux; Android 11; Redmi K20 Pro Build/RP1A.201105.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/78.0.3904.62 XWEB/2693 MMWEBSDK/201101 Mobile Safari/537.36 MMWEBID/807 MicroMessenger/7.0.21.1800(0x2700153B) Process/toolsmp WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64")
	req.Header.Add("X-Requested-With","com.tencent.mm")
	req.Header.Add("Referer","http://wx.fjpit.com/wap/index.html?v=2.1")
	return req
}

func getResp(client *http.Client,req *http.Request) (string,int,error) {
	resp,err := client.Do(req)
	if err != nil {
		return empty,-1,err
	}
	defer resp.Body.Close()
	resBytes,err2 := ioutil.ReadAll(resp.Body)
	if err2 != nil {
		return empty,-1,err
	}
	return string(resBytes),resp.StatusCode,nil
}

func urlRequest(mode string,destUrl string,data io.Reader) (string,int,error) {
	var count int
	client := &http.Client {
		Jar: cookieJar,
		Timeout: 500 * time.Millisecond,
	}
		//fmt.Println("Post:",destUrl)
		//req := newRequest(mode,destUrl,data)
		//loadCookies(req)
		for {
			count +=1
			if count >1 {
				fmt.Printf("Retry.. Count: %d\n",count-1)
			}
			resp,code,err := getResp(client,newRequest(mode,destUrl,data))
			if err != nil{
				if err.(net.Error).Timeout(){
					if !retry{
						return resp,code,err
					}
					if count > retryMAX{
						break
					}
					time.Sleep(time.Duration(sleep) * time.Second)

					continue
				}else {
					return resp,code,err
				}
			}else {
				return resp,code,nil
			}
		}
	return empty, -1, errors.New("max retry counts reached")
}

func checkStatusCode(code int,desire int) error{
	if code != desire{
		return errors.New("status code check failed")
	}
	return nil
}

//func checkToken(){
//	if len(token) == 0{
//		check(errors.New("token not found"))
//	}
//}

func setPayload(t string){
	token = t
	payload = strings.NewReader(url.Values{"opt": []string{"1"},"token": []string{token}}.Encode())
}

func getCourses() []COURSE {
	var courses []COURSE
	res,code,err := urlRequest(post,apiCourseList,payload)
	check(true,err,checkStatusCode(code,httpOK))
	dataset := DATAs{}
	check(true,json.Unmarshal([]byte(res),&dataset))
	for _,v := range dataset.Data.Wrapper {
		courses = append(courses, v)
	}
	return courses
}

func postCourse(course COURSE){
	start := time.Now()
	defer wg.Done()
	for {
		fmt.Printf("ID: %s 课程:%s 开始选课..\n",course.Id,course.Name)
		tp := strings.NewReader(url.Values{"opt": []string{"1"},"token": []string{token},"xkxxid": []string{course.Id}}.Encode())
		res,code,err := urlRequest(post,apiPostCourse,tp)
		if err != nil{
			continue
		}
		check(true,checkStatusCode(code,httpOK))
		reply := REPLY{}
		check(true,json.Unmarshal([]byte(res),&reply))
		// we don't know the status code of succeed
		if checkStatusCode(reply.Status, errorCode) != nil{
			if strings.ContainsAny(reply.Msg,"成功") || strings.ContainsAny(reply.Msg,"恭喜") {
				fmt.Printf("ID: %s 似乎选上了!!\n",course.Id)
				break
		}
		// error handlers
		}else if strings.ContainsAny(reply.Msg,"已满"){
			fmt.Printf("ID: %s 人数已满.\n",course.Id)
			break
		}else if strings.ContainsAny(reply.Msg,"超过选课数量") {
			fmt.Println("看起来你似乎已经好课了嘛，把名额留给别人吧~")
			break
			// attempt to retry
		} else if retry{
			fmt.Printf("错误:%s 等待 %d 秒后重试..\n", reply.Msg,sleep)
			time.Sleep(time.Duration(sleep) * time.Second)
		}
		//fmt.Printf("%+v\n",reply)
	}
	fmt.Printf("took: %s\n",time.Since(start))
}

func postCourses(courses []COURSE){
	wg.Add(len(courses))
	for _,v := range courses{
		if getAvailability(v){
			go postCourse(v)
		}else {
			fmt.Printf("ID: %s 课程: %s 不可选\n",v.Id,v.Name)
			wg.Done()
		}
	}
	fmt.Println("选课并发控制进程结束..")
}

func getAvailability(course COURSE)  bool{
	if course.Max == empty {
		return true
	}
	ordered,_ := strconv.Atoi(course.Ordered)
	max,_ := strconv.Atoi(course.Max)
	if ordered >= max {
		return false
	}
	return true
}

func appRun(c *cli.Context) error{
	fmt.Println("欢迎使用由 Starx 制作的自动选课程序。本程序仅可用于[福建信息职业技术学院]的选课平台。\n本程序制作的初衷是为了让大家更容易抢到课，而不需要跟辣鸡选课平台斗智斗勇浪费时间。\n本程序当前为非盈利性质程序，严禁未经授权的倒卖、修改、出售。\n希望大家都能抢到自己喜欢的课程。")
	argToken  := c.String("token")
	argId	  := c.String("id")
	retry 	   = c.Bool("retry")
	sleep = c.Int("second")
	//
	setPayload(argToken)
	if c.Bool("list") {
		var ids map[string]string
		fmt.Println("获取选修课列表开始...")
		courses := getCourses()
		for i,v := range courses {
			status := available
			if getAvailability(v) {
				if ids == nil{
					ids = map[string]string{}
				}
				ids[v.Id] = v.Name
				}else {
					status = full
				}
				fmt.Printf("序列: %d 状态: %s ID: %s 课程: %s 教师: %s  学分: %s 班级: %s 校区: %s\n",i,status,v.Id,v.Name,v.Teacher,v.Point,v.Class,v.Block)
			}
			fmt.Println("选修课列表输出结束.")
			if len(ids) > 0 {
				var idsA []string
				fmt.Print("当前可用:")
				for k,v := range ids{
					idsA = append(idsA, k)
					fmt.Printf(" %s[%s]",v,k)
				}
				fmt.Printf("\n集合: %s\n",strings.Join(idsA,","))
			}else {
				fmt.Println("当前暂无可用选修课..")
			}
		}else{
			if argId == all {
				fmt.Println("默认选取所有可用课程..")
				postCourses(getCourses())
				wg.Wait()
			}else {
				ids := strings.Split(argId,",")
				courses := getCourses()
				var tmp []COURSE
				for _,v := range courses{
					for _,id := range ids{
						if id != v.Id{
							continue
						}else {
							tmp = append(tmp, v)
						}
					}
				}
				postCourses(tmp)
				wg.Wait()
			}
		}
	fmt.Println("程序结束..")
	return nil
}

func main() {
	app := &cli.App{
		Name:        "Auto FJPIT Course Picker",
		HelpName:    "acp",
		Usage:       "Automatic pick the courses you preferred of FJPIT.",
		UsageText:   "./acp --token your_token --id your_course_id[,id...]",
		Version:     "v1.0",
		Flags:       []cli.Flag{&cli.StringFlag{
			Name:     "token",
			Aliases:  []string{"t"},
			Usage:    "the token of your own account",
			Required: true,
		},&cli.StringFlag{
			Name:     "id",
			Aliases:  []string{"i"},
			Usage:    "the id of course[s] you preferred. split by [,] if multi valued.",
			Required: false,
			Value: "all",
		},&cli.BoolFlag{
			Name:     "retry",
			Aliases:  []string{"r"},
			Usage:    "keeping retry if not succeed",
			Required: false,
			Value: true,
		},&cli.IntFlag{
			Name:     "second",
			Aliases:  []string{"s"},
			Usage:    "the retry interval seconds.",
			Required: false,
			Value: 1,
		},&cli.BoolFlag{
			Name:     "list",
			Aliases:  []string{"l"},
			Usage:    "list all available course[s] on your account.",
			Required: false,
			Value: false,
		}},
		Action:      appRun,
		Copyright:	author,
	}
	err := app.Run(os.Args)
	check(true,err)
}
