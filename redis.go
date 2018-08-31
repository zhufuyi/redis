package redis

import (
	"fmt"

	"github.com/zhufuyi/logger"

	"time"

	"github.com/gomodule/redigo/redis"
	"go.uber.org/zap"
)

// 当前应用允许最大连接数，最大不能超过redis极限连接数
// 如果多个项目共用同一个redis，要考虑每个项目限制连接数，防止过量连接造成redis卡死
var maxActiveCount = 2800 // 经过压测得到大概结果

var pool *RedisPool

// GetConn, 对redis操作前，先要调用此函数从redis连接池获取一个连接，要对结果做错误判断，防止获取nil的连接，然后对空指针操作造成panic
func GetConn() (RedisConn, error) {
	if pool == nil {
		logger.Panic("redis pool is nil, go to connect redis first, eg: redis.NewRedisPool(server, password string)")
	}

	// 超出redis承受的极限最大连接数，直接拦截，并返回错误
	if pool.ActiveCount() > maxActiveCount {
		return nil, fmt.Errorf("redis connect clients exceeded the limit of %d", maxActiveCount)
	}

	return pool.Get(), nil
}

type RedisPool struct {
	redis.Pool
}

func (r *RedisPool) Get() RedisConn {
	conn := r.Pool.Get()
	return &DefaultRedisConn{Conn: conn}
}

type RedisConn interface {
	redis.Conn
	WithLog() RedisConn
}

type DefaultRedisConn struct {
	redis.Conn
	printLog bool
}

func (d *DefaultRedisConn) WithLog() RedisConn {
	d.printLog = true
	return d
}

func (d *DefaultRedisConn) Do(commandName string, args ...interface{}) (reply interface{}, err error) {
	result, err := d.Conn.Do(commandName, args...)
	if err != nil {
		if d.printLog {
			d.printLog = false
			printError(err, "redis do error", commandName, args...)
		}
		return result, err
	}

	if d.printLog {
		d.printLog = false
		printInfo(result, "redis do", commandName, args...)
	}

	return result, err
}

// Send writes the command to the client's output buffer.
func (d *DefaultRedisConn) Send(commandName string, args ...interface{}) error {
	err := d.Conn.Send(commandName, args...)
	if err != nil {
		if d.printLog {
			d.printLog = false
			printError(err, "redis send error", commandName, args...)
		}
		return err
	}

	if d.printLog {
		d.printLog = false
		printInfo(nil, "redis send", commandName, args...)
	}

	return err
}

// Flush flushes the output buffer to the Redis server.
func (d *DefaultRedisConn) Flush() error {
	err := d.Conn.Flush()
	if err != nil {
		if d.printLog {
			d.printLog = false
			printError(err, "redis Flush error", "")
		}
		return err
	}

	return nil
}

// Receive receives a single reply from the Redis server
func (d *DefaultRedisConn) Receive() (reply interface{}, err error) {
	result, err := d.Conn.Receive()
	if err != nil {
		if d.printLog {
			d.printLog = false
			printError(err, "redis receive error", "")
		}
		return result, err
	}

	if d.printLog {
		d.printLog = false
		printInfo(result, "redis receive", "")
	}
	return result, err
}

// 转换类型
func anyField(key string, a interface{}) zap.Field {
	switch a.(type) {
	case []byte:
		return logger.String(key, string(a.([]byte)))

	case string:
		return logger.String(key, a.(string))

	case []interface{}:
		value := []string{}
		for _, v := range a.([]interface{}) {
			if _, ok := v.([]byte); ok {
				value = append(value, fmt.Sprintf("%s", v))
			}
		}
		return logger.Any(key, value)

	default:
		return logger.String(key, fmt.Sprintf("%v", a))
	}
}

func printError(err error, msg string, commandName string, args ...interface{}) {
	if commandName == "" {
		logger.Error(msg, logger.Err(err))
		return
	}

	logger.Error(msg,
		logger.Err(err),
		logger.String("commandName", commandName),
		logger.Any("args", args),
	)
}

func printInfo(result interface{}, msg string, commandName string, args ...interface{}) {
	// commandName为空，说明打印receiver函数返回的结果
	if commandName == "" {
		logger.WithFields(anyField("result", result)).Info(msg)
		return
	}

	commandArgs := commandName
	for _, arg := range args {
		commandArgs += fmt.Sprintf(" %v", arg)
	}
	// result为空，说明打印send函数的命令和参数
	if result == nil {
		logger.WithFields(zap.String("command", commandArgs)).Info("redis send")
	}

	// 打印do函数的命令、参数、结果
	logger.WithFields(zap.String("command", commandArgs), anyField("result", result)).Info("redis do")
}

// NewRedisPool connect redis，if test ping failed，return error
func NewRedisPool(server, password string) error {
	pool = &RedisPool{
		Pool: redis.Pool{
			MaxIdle:     3,
			IdleTimeout: 240 * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial("tcp", server)
				if err != nil {
					return nil, err
				}

				if _, err = c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}

				c.Do("select", 0)
				return c, err
			},

			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, err := c.Do("PING")
				return err
			},
		},
	}

	rconn, _ := GetConn()
	defer rconn.Close()

	_, err := rconn.Do("PING")
	return err
}

// ----------------------- 重新包装函数 --------------------------

func Int(reply interface{}, err error) (int, error) {
	return redis.Int(reply, err)
}

func Int64(reply interface{}, err error) (int64, error) {
	return redis.Int64(reply, err)
}

func Uint64(reply interface{}, err error) (uint64, error) {
	return redis.Uint64(reply, err)
}

func Float64(reply interface{}, err error) (float64, error) {
	return redis.Float64(reply, err)
}

func String(reply interface{}, err error) (string, error) {
	return redis.String(reply, err)
}

func Bytes(reply interface{}, err error) ([]byte, error) {
	return redis.Bytes(reply, err)
}

func Bool(reply interface{}, err error) (bool, error) {
	return redis.Bool(reply, err)
}

func Values(reply interface{}, err error) ([]interface{}, error) {
	return redis.Values(reply, err)
}

func Strings(reply interface{}, err error) ([]string, error) {
	return redis.Strings(reply, err)
}

func ByteSlices(reply interface{}, err error) ([][]byte, error) {
	return redis.ByteSlices(reply, err)
}

func Ints(reply interface{}, err error) ([]int, error) {
	return redis.Ints(reply, err)
}

func StringMap(result interface{}, err error) (map[string]string, error) {
	return redis.StringMap(result, err)
}

func IntMap(result interface{}, err error) (map[string]int, error) {
	return redis.IntMap(result, err)
}

func Int64Map(result interface{}, err error) (map[string]int64, error) {
	return redis.Int64Map(result, err)
}
