package main

// TODO 将确认所有权的部分做成中间件
// folderID -> folder
// requestID -> request

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"strconv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// for testing
	"gorm.io/driver/sqlite"
)

// config
const IS_UPDATE = false
const dbType = "postgres" // postgres or sqlite
const TOKEN_PERIOD = 1800
var Entities = []any{&User{},&Folder{},&Request{},&Parameter{}} // 用于数据库migrate

func corsConfig(ginServer *gin.Engine) {
	mwCORS := cors.New(cors.Config{
		//准许跨域请求网站,多个使用,分开,限制使用*
		AllowOrigins:     []string{"*"},
		//准许使用的请求方式
		AllowMethods:     []string{"PUT", "PATCH", "POST", "GET", "DELETE"},
		//准许使用的请求表头
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type"},
		//显示的请求表头
		ExposeHeaders:    []string{"Content-Type"},
		//凭证共享,确定共享
		AllowCredentials: true,
		//容许跨域的原点网站,可以直接return true就万事大吉了
		AllowOriginFunc: func(origin string) bool {
			 return true
		},
		//超时时间设定
		MaxAge: 24 * time.Hour,
 })
 ginServer.Use(mwCORS)
}

// models

type ParamType int

const (
	Header ParamType = 0
	Query ParamType = 1
	BodyWithFormData ParamType = 2
	BodyWithJson ParamType = 3
)

type methodType string

const (
	GET methodType = "GET"
	POST methodType = "POST"
	PUT methodType = "PUT"
	DELETE methodType = "DELETE"
	PATCH methodType = "PATCH"
	HEAD methodType = "HEAD"
	CONNECT methodType = "CONNECT"
	OPTIONS methodType = "OPTIONS"
	TRACE methodType = "TRACE"
)

type protocolType string

const (
	HTTP protocolType = "http://"
	HTTPS protocolType = "https://"
)

// 定义model
type Parameter struct {
	ID uint
	Type ParamType
	Key string
	Value string
	// description string
	RequestID uint
}

type Request struct {
	ID uint
	Parameters []Parameter
	FolderID uint
	UserID string
	Name string
	Url string
	Result string
	Method methodType
	ProtocolHeader protocolType
}

type Folder struct {
	ID uint
	Name string
	Requests []Request
	UserID string
}

type User struct {
	ID string
	Name string
	Folders []Folder
	Requests []Request
	Password string `mapstructure:",omitempty"`
}

// for sqlite

func SqliteConnect() (db *gorm.DB) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
  if err != nil {
    panic("failed to connect database")
  }
	return db
}

// for postgres
func DBConnect(host string,user string, passwordKey string, dbName string,port string) (db *gorm.DB) {
	err := godotenv.Load("local.env")
	if err != nil {
		fmt.Println("env load err:",err)
		return
	}
	dsn := "host="+host+" user="+user+" password="+ os.Getenv(passwordKey)+" dbname="+dbName+" port="+port+" sslmode=disable TimeZone=Asia/Shanghai"
	db, err = gorm.Open(postgres.New(postgres.Config{
		DSN: dsn,
		PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}), &gorm.Config{})
	if err != nil {
		fmt.Println("db connect err:",err)
		return
	}
	return
}

func DBUpdate(isUpdate bool,db *gorm.DB, models []any) {
	if isUpdate {
		// 将模型迁移到数据库
		fmt.Println("model migrate")
		err := db.AutoMigrate(models...)
		if err != nil {
			fmt.Println("",err)
			return
		}
	} else {
		fmt.Println("model migration skiped")
	}
}

// 帮助函数

func Uint64ToUint(num uint64 ,err error) (uint ,error) {
		return uint(num), err
}

// 中间件

func Authenticate(ctx *gin.Context) {
	// TODO 添加用户在数据库存在的验证
	// @params in header Authorization
	// @notice 验证用户JWT
	// @return 在ctx中set user， 值为用户ID（sub字段）
	tokenStr := ctx.Request.Header["Authorization"][0]
	res := jwt.MapClaims{}
	token,err := jwt.ParseWithClaims(tokenStr,res,func(t *jwt.Token) (interface{}, error) {return []byte(os.Getenv("JWT_KEY")),nil})
	if err != nil {
		ctx.AbortWithError(401,err)
		return
	}
	if !token.Valid {
		ctx.AbortWithError(401,errors.New("invalid token"))
		return
	}
	ctx.Set("user",res["sub"])
}

var requestOwned func(*gin.Context)
var folderOwned func(*gin.Context)

func setMiddleWare(db *gorm.DB) {
	requestOwned = func(ctx *gin.Context) {
		// @params in query requestId
		// @params in context user
		// @notice 验证是否有这个request，request是否属于该账户
		// @return in context request requestId

		// 获取id
		id,err:=Uint64ToUint(strconv.ParseUint(ctx.Query("requestId"),0,32))
		if err != nil {
			ctx.JSON(400,gin.H{"msg":"id解析错误","err":err})
			return
		}
		// 获取request
		var request Request
		var res *gorm.DB
		// 根据fullPath分别处理
		switch ctx.FullPath() + " " + ctx.Request.Method {
			case "/request GET":
				res = db.Preload("Parameters").First(&request,id)
			default:
				res = db.First(&request,id)
		}
		if res.Error != nil {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":"查找请求错误","err":res.Error})
			return
		}
		// 验证所有权
		if request.UserID != ctx.GetString("user") {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":"不属于该账户的请求"})
			return 
		}
		ctx.Set("request",request)
		ctx.Set("requestId",id)
	}
	folderOwned = func(ctx *gin.Context) {
		// @params in query folderId
		// @params in context user
		// @notice 验证是否有这个folder，folder是否属于该账户
		// @return in context folder folderId
		id,err:=Uint64ToUint(strconv.ParseUint(ctx.Query("folderId"),0,32))
		if err != nil {
			ctx.JSON(400,gin.H{"msg":"id解析错误","err":err})
			return
		}
		var folder Folder
		var res *gorm.DB
		switch ctx.FullPath() + " " + ctx.Request.Method {
			case "/folder GET":
				res =db.Preload("Requests").First(&folder,id)
			default:
				res = db.First(&folder,id)
		}
		if res.Error != nil {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":"查找请求错误","err":res.Error})
			return
		}
		// 验证所有权
		if folder.UserID != ctx.GetString("user") {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":"不属于该账户的文档"})
			return 
		}
		ctx.Set("folder",folder)
		ctx.Set("folderId",folder)
	}
}

// api

func PingApi(ginServer *gin.Engine,db *gorm.DB) {
	ginServer.GET("/ping",func(ctx *gin.Context) {
		// @params no param
		// @notice 保证服务器畅通，开发过程中可以用来做测试
		ctx.JSON(200,"pong")
	})
}

func RequestApi(ginServer *gin.Engine,db *gorm.DB) {
	ginServer.GET("/request",Authenticate,requestOwned,func (ctx *gin.Context) {
		// @params in query requestId
		// @notice 获取请求
		
		var request = ctx.MustGet("request")
		ctx.JSON(200,request)
	})


	ginServer.POST("/request",Authenticate,func (ctx *gin.Context) {
		// @params in body {Name,Url,FolderID,ProtocolHeader,Method,Parameters(header,query,body)}
		// @notice 添加请求，
		var request Request
		if err := ctx.BindJSON(&request);err != nil {
			fmt.Println("bind err:",err)
			return
		}
		var folder Folder
		if res := db.First(&folder,request.FolderID);res.Error != nil {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":res.Error.Error()})
			return 
		}
		// 验证所有权
		if folder.UserID != ctx.GetString("user") {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":"不能将请求加入不属于账号的文件夹"})
			return 
		}
		request.UserID = ctx.GetString("user")
		if res := db.Create(&request);res.Error != nil {
			fmt.Println("create err:",res.Error)
			return
		}

	})
	ginServer.PATCH("/request",Authenticate,func(ctx *gin.Context) {
		// @params in body {id,url,protocolHeader,method,parameters(header,query,body),Result}
		// @notice 修改请求，只能修改请求内容，不能修改请求所属账号和请求所在文件夹
		var request Request
		if err := ctx.BindJSON(&request);err != nil {
			fmt.Println("bind err:",err)
			return
		}

		var oldRequest = Request{}
		res := db.First(&oldRequest,request.ID)
		if res.Error != nil {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":res.Error.Error()})
			return
		}
		// 验证所有权
		if oldRequest.UserID != ctx.GetString("user") {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":"修改不属于该账户的请求"})
			return
		}
		// 修改
		oldRequest.Url = request.Url
		oldRequest.ProtocolHeader = request.ProtocolHeader
		oldRequest.Method = request.Method
		oldRequest.Parameters = request.Parameters
		oldRequest.Result = request.Result
		res = db.Save(oldRequest)
		if res.Error != nil {
			ctx.JSON(http.StatusBadRequest,gin.H{"msg":res.Error.Error()})
			return
		}
		ctx.JSON(200,oldRequest)
	})
	ginServer.DELETE("/request",Authenticate,requestOwned,func(ctx *gin.Context) {
		// @params in query requestId
		// @notice 删除请求
		id := ctx.GetUint("requestId")
		res := db.Select("Parameters").Delete(&Request{},id)
		if res.Error != nil {
			ctx.JSON(400,gin.H{"msg":gin.H{"msg":"删除错误","err":res.Error}})
			return
		}
		ctx.JSON(204,nil)
	})
	ginServer.PATCH("/request/rename",Authenticate,requestOwned,func(ctx *gin.Context) {
		// @params in query requestId,name
		// @notice 为重命名多一个接口来优化性能
		requestAny := ctx.MustGet("request")
		request,ok := requestAny.(Request)
		if !ok {
			ctx.JSON(http.StatusInternalServerError,gin.H{"msg":"请求转换错误"})
		}

		request.Name = ctx.Query("name")
		db.Save(request)
		ctx.JSON(200,gin.H{"msg":"重命名完成"})
	})
	ginServer.PATCH("/request/transfer",Authenticate,requestOwned,folderOwned,func(ctx *gin.Context) {
		// @params in query requestId folderId
		// @notice 处理请求在文件夹间的移动
		requestAny := ctx.MustGet("request")
		request,ok := requestAny.(Request)
		if !ok {
			ctx.JSON(http.StatusInternalServerError,gin.H{"msg":"请求转换错误"})
		}
		folderId := ctx.GetUint("folderId")
		request.FolderID = folderId
		db.Save(request)
		ctx.JSON(200,gin.H{"msg":"转移完成"})
	})
}

func FolderApi(ginServer *gin.Engine, db *gorm.DB) {
	ginServer.GET("/folder",Authenticate,folderOwned,func(ctx *gin.Context) {
		// @params in query folderId
		// @notice 获取文件夹以及文件夹中的所有请求的部分信息
		folderAny := ctx.MustGet("folder")
		folder,ok := folderAny.(Folder)
		if !ok {
			ctx.JSON(http.StatusInternalServerError,gin.H{"msg":"文件夹转换错误"})
		}
		ctx.JSON(http.StatusAccepted,folder)
	})
	ginServer.POST("/folder",Authenticate,func(ctx *gin.Context) {
		// @params in query name
		// @notice 新建空文件夹
		// @return 返回空文件夹

		folder := Folder{Name: ctx.Query("name"),UserID: ctx.GetString("user")}
		res :=db.Create(&folder)
		if res.Error != nil {
			fmt.Println("folder Create err:",res.Error)
			return
		}
		ctx.JSON(200,folder)
	})

	ginServer.PATCH("/folder",Authenticate,folderOwned,func(ctx *gin.Context) {
		// @params in query folderId Name
		// @notice 改变文件夹名字
		// @return 返回新的文件夹元数据
		name := ctx.Query("Name")
		folderAny := ctx.MustGet("folder")
		folder,ok := folderAny.(Folder)
		if !ok {
			ctx.JSON(http.StatusInternalServerError,gin.H{"msg":"文件夹转换错误"})
		}
		folder.Name = name
		db.Save(&folder)
		ctx.JSON(200,folder)
	})

	ginServer.DELETE("/folder",Authenticate,func(ctx *gin.Context) {
		// @params in query folderId
		// @notice 删除文件夹以及文件夹中的请求
		// @return 返回204 no content
		id := ctx.GetUint("folderId")
		err := db.Unscoped().Model(&Folder{ID: id}).Association("Requests").Unscoped().Clear()
		if err != nil {
			fmt.Println("associate delete err:",err)
			return
		}
		db.Delete(&Folder{},id)
		ctx.JSON(http.StatusNoContent,nil)
	})
}


func UserApi(ginServer *gin.Engine , db *gorm.DB) {
	ginServer.POST("/login",func(ctx *gin.Context) {
		// @params in body ID,Password
		// @notice 用户登录
		// @return token或错误
		var user User
		if err := ctx.BindJSON(&user);err != nil {
			ctx.JSON(400,gin.H{"json err":err})
			return
		}
		password := user.Password
		res := db.First(&user,"id = ?",user.ID)
		if res.Error != nil {
			ctx.JSON(400,gin.H{"query err":res.Error})
			return
		}
		if password != user.Password {
			// delete
			ctx.JSON(400,gin.H{"msg":"密码不匹配"})
			return
		}
		tokenValue := jwt.MapClaims{
			"sub":user.ID,
			"exp":time.Now().Unix()+ TOKEN_PERIOD,
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256,tokenValue)
		tokenStr,err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
		if err != nil {
			ctx.JSON(400,gin.H{"msg":"token生成失败","err":err.Error()})
			return
		}
		ctx.JSON(200,gin.H{"token":tokenStr})
	})

	ginServer.POST("/user",func(ctx *gin.Context) {
		// @params in body Name,Password,ID
		// @notice 用户注册
		// @return 错误或空
		var user User
		if err := ctx.BindJSON(&user);err != nil {
			fmt.Println("BindJson err:",err)
			ctx.JSON(400,gin.H{"msg":err})
			return
		}
		if res := db.Create(user);res.Error != nil {
			fmt.Println("Create user err:",res.Error)
			ctx.JSON(400,gin.H{"msg":res.Error})
			return
		}
		ctx.JSON(200,gin.H{"msg":"创建成功"})
	})

	ginServer.PATCH("/user",Authenticate,func(ctx *gin.Context) {
		// @params in query Name
		// @notice 修改用户名
		// @return 204或错误
		userId := ctx.GetString("user")
		user := User{}
		res := db.First(&user,"id = ?",userId)
		if res.Error != nil {
			ctx.JSON(400,gin.H{"msg":"userNotFound","err":res.Error})
			return
		}
		user.Name = ctx.Query("Name")
		db.Save(user)
		ctx.JSON(204,nil)
		
	})

	ginServer.GET("/user",Authenticate,func(ctx *gin.Context) {
		// @params no param
		// @notice 获得用户信息
		// @return 用户信息或错误（未找到）
		user := ctx.GetString("user")
		userInfo := User{}
		res := db.Preload("Folders.Requests.Parameters").First(&userInfo,"id = ?",user)
		if res.Error != nil {
			ctx.JSON(400,gin.H{"msg":res.Error})
			return
		}
		userInfo.Password = "" // 隐藏密码
		ctx.JSON(200,userInfo)
	})
}

func GinSet(dbType string) *gin.Engine {
	ginServer := gin.Default()
	var db *gorm.DB

	// cors
	corsConfig(ginServer)
	// 链接数据库
	if dbType == "postgres" {	
		db = DBConnect("localhost","postgres","POSTGREPASS","apitest","5432")
	} else if dbType == "sqlite" {
		db = SqliteConnect()
	}
	
	DBUpdate(IS_UPDATE,db,Entities)
	// middleware
	setMiddleWare(db)
	// api

	//ping
	PingApi(ginServer,db)

	// request
	RequestApi(ginServer,db)

	// folder
	FolderApi(ginServer,db)
	
	// user
	UserApi(ginServer,db)
	return ginServer
}

func main() {
	ginServer := GinSet(dbType)
	ginServer.Run("localhost:8082")
}