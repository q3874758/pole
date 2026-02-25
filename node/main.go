package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ==================== 类型定义 ====================

type Config struct {
	NodeName    string `yaml:"node_name"`
	Network     string `yaml:"network"`
	DataDir     string `yaml:"data_dir"`
	LogLevel    string `yaml:"log_level"`
	APIEnabled  bool   `yaml:"api_enabled"`
	APIPort     int    `yaml:"api_port"`
	SteamAPIKey string `yaml:"steam_api_key"`
}

type Node struct {
	ID        string
	Name      string
	Address   string
	Stake     uint64
	Reputation float64
	Uptime    float64
	Status    string
}

type GameData struct {
	GameID       string `json:"game_id"`
	OnlinePlayers uint64 `json:"online_players"`
	PeakPlayers  uint64 `json:"peak_players"`
	Timestamp    int64  `json:"timestamp"`
	Tier         int    `json:"tier"`
}

type GVS struct {
	GameID  string  `json:"game_id"`
	Score   float64 `json:"score"`
	Tier    int     `json:"tier"`
	Updated int64   `json:"updated"`
}

type NetworkStats struct {
	Height         uint64         `json:"height"`
	Epoch          uint64         `json:"epoch"`
	TotalNodes     int            `json:"total_nodes"`
	ActiveNodes    int            `json:"active_nodes"`
	TotalGVS       float64        `json:"total_gvs"`
	BlockTime      float64        `json:"block_time"`
}

// ==================== 全局变量 ====================

var (
	config     Config
	node       Node
	gvsMap     = make(map[string]GVS)
	gameData   = make(map[string][]GameData)
	networkStats NetworkStats
	stopChan   = make(chan bool)
	startTime  time.Time
)

// ==================== 初始化 ====================

func init() {
	startTime = time.Now()
	
	// 生成节点ID
	hash := sha256.Sum256([]byte(time.Now().String()))
	node.ID = hex.EncodeToString(hash[:])[:16]
	node.Status = "active"
	node.Stake = 0
	node.Reputation = 100.0
	node.Uptime = 100.0
	
	// 默认配置
	config = Config{
		NodeName:   "PoLE-Node-" + node.ID[:8],
		Network:    "testnet",
		DataDir:    "./data",
		LogLevel:   "info",
		APIEnabled: true,
		APIPort:    8080,
	}
	
	networkStats = NetworkStats{
		Height:      0,
		Epoch:       1,
		TotalNodes:  1,
		ActiveNodes: 1,
	}
	
	// 创建数据目录
	os.MkdirAll(config.DataDir, 0755)
}

// ==================== 日志 ====================

func logInfo(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

func logWarn(format string, args ...interface{}) {
	fmt.Printf("[WARN] "+format+"\n", args...)
}

func logError(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

// ==================== 核心功能 ====================

func startNode() {
	logInfo("========================================")
	logInfo("  PoLE 节点启动")
	logInfo("========================================")
	logInfo("节点 ID: %s", node.ID)
	logInfo("节点名称: %s", config.NodeName)
	logInfo("网络: %s", config.Network)
	logInfo("数据目录: %s", config.DataDir)
	logInfo("========================================")
	
	// 启动数据采集
	go collectGameData()
	
	// 启动GVS计算
	go calculateGVS()
	
	// 启动区块生产模拟
	go produceBlocks()
	
	// 启动API服务器
	if config.APIEnabled {
		go startAPIServer()
	}
	
	// 等待信号
	waitForSignal()
}

func collectGameData() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	// 热门游戏列表 (Steam App IDs)
	games := []string{
		"730",    // CS2
		"570",    // Dota 2
		"440",    // Team Fortress 2
		"292030", // Witcher 3
		"1091500",// Cyberpunk 2077
		"814380", // Sekiro
		"1551360",// Forza Horizon 5
		"1245620",// Elden Ring
		"271590", // GTA V
	}
	
	for {
		select {
		case <-ticker.C:
			for _, gameID := range games {
				data := fetchSteamData(gameID)
				if data.GameID != "" {
					gameData[gameID] = append(gameData[gameID], data)
					// 只保留最近100条数据
					if len(gameData[gameID]) > 100 {
						gameData[gameID] = gameData[gameID][1:]
					}
					logInfo("采集到游戏 %s 数据: %d 在线", data.GameID, data.OnlinePlayers)
				}
			}
		case <-stopChan:
			return
		}
	}
}

func fetchSteamData(appID string) GameData {
	url := fmt.Sprintf("https://api.steampowered.com/ISteamUserStats/GetNumberOfCurrentPlayers/v1/?appid=%s", appID)
	
	resp, err := http.Get(url)
	if err != nil {
		return GameData{}
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return GameData{}
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return GameData{}
	}
	
	// 简单解析JSON
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return GameData{}
	}
	
	response, ok := result["response"].(map[string]interface{})
	if !ok {
		return GameData{}
	}
	
	playerCount, _ := response["player_count"].(float64)
	
	return GameData{
		GameID:       appID,
		OnlinePlayers: uint64(playerCount),
		PeakPlayers:  uint64(playerCount),
		Timestamp:    time.Now().Unix(),
		Tier:         1,
	}
}

func calculateGVS() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			var totalGVS float64
			
			for gameID, dataPoints := range gameData {
				if len(dataPoints) == 0 {
					continue
				}
				
				// 计算GVS
				var avgPlayers float64
				for _, dp := range dataPoints {
					avgPlayers += float64(dp.OnlinePlayers)
				}
				avgPlayers /= float64(len(dataPoints))
				
				// GVS = log(avg_players + 1) * sqrt(peak) * tier_weight * time_decay
				tierWeight := 1.0
				if len(dataPoints) > 0 {
					switch dataPoints[len(dataPoints)-1].Tier {
					case 1:
						tierWeight = 1.0
					case 2:
						tierWeight = 0.45
					case 3:
						tierWeight = 0.10
					}
				}
				
				// 时间衰减
				timeDecay := 1.0
				if len(dataPoints) > 0 {
					lastTime := dataPoints[len(dataPoints)-1].Timestamp
					ageHours := float64(time.Now().Unix()-lastTime) / 3600
					timeDecay = 1.0 / (1.0 + ageHours/24.0)
				}
				
				gvs := (avgPlayers + 1) * tierWeight * timeDecay
				
				gvsMap[gameID] = GVS{
					GameID:  gameID,
					Score:   gvs,
					Tier:    dataPoints[len(dataPoints)-1].Tier,
					Updated: time.Now().Unix(),
				}
				
				totalGVS += gvs
			}
			
			networkStats.TotalGVS = totalGVS
			logInfo("GVS 计算完成: 总GVS = %.2f", totalGVS)
			
		case <-stopChan:
			return
		}
	}
}

func produceBlocks() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			networkStats.Height++
			if networkStats.Height%100 == 0 {
				networkStats.Epoch++
			}
			
			if networkStats.Height%10 == 0 {
				logInfo("区块高度: %d, Epoch: %d", networkStats.Height, networkStats.Epoch)
			}
			
		case <-stopChan:
			return
		}
	}
}

// ==================== API 服务器 ====================

func startAPIServer() {
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/node", handleNodeInfo)
	http.HandleFunc("/api/gvs", handleGVS)
	http.HandleFunc("/api/games", handleGames)
	http.HandleFunc("/api/stats", handleStats)
	
	addr := fmt.Sprintf(":%d", config.APIPort)
	logInfo("API 服务器启动: http://localhost%s", addr)
	
	if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed {
		logError("API 服务器错误: %v", err)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "running",
		"node_id":   node.ID,
		"uptime":    time.Since(startTime).String(),
		"height":    networkStats.Height,
		"timestamp": time.Now().Unix(),
	})
}

func handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(node)
}

func handleGVS(w http.ResponseWriter, r *http.Request) {
	gameID := r.URL.Query().Get("game_id")
	
	if gameID != "" {
		if gvs, ok := gvsMap[gameID]; ok {
			json.NewEncoder(w).Encode(gvs)
			return
		}
		http.Error(w, "Game not found", 404)
		return
	}
	
	// 返回所有GVS
	list := make([]GVS, 0, len(gvsMap))
	for _, g := range gvsMap {
		list = append(list, g)
	}
	json.NewEncoder(w).Encode(list)
}

func handleGames(w http.ResponseWriter, r *http.Request) {
	gameID := r.URL.Query().Get("game_id")
	
	if gameID != "" {
		if data, ok := gameData[gameID]; ok {
			json.NewEncoder(w).Encode(data)
			return
		}
		http.Error(w, "Game not found", 404)
		return
	}
	
	// 返回所有游戏数据
	list := make([]map[string]interface{}, 0, len(gameData))
	for id, data := range gameData {
		list = append(list, map[string]interface{}{
			"game_id":  id,
			"count":    len(data),
			"latest":   data[len(data)-1],
		})
	}
	json.NewEncoder(w).Encode(list)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(networkStats)
}

// ==================== 命令行交互 ====================

func startInteractive() {
	logInfo("========================================")
	logInfo("  PoLE 节点控制台")
	logInfo("========================================")
	logInfo("输入 'help' 查看命令")
	logInfo("========================================")
	
	scanner := bufio.NewScanner(os.Stdin)
	
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		
		handleCommand(line)
	}
}

func handleCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}
	
	cmd := parts[0]
	
	switch cmd {
	case "help":
		printHelp()
	case "status":
		printStatus()
	case "gvs":
		printGVS()
	case "games":
		printGames()
	case "stats":
		printStats()
	case "set":
		if len(parts) > 2 {
			setConfig(parts[1], strings.Join(parts[2:], " "))
		} else {
			logInfo("用法: set <key> <value>")
		}
	case "exit", "quit":
		stopChan <- true
		logInfo("节点已停止")
		os.Exit(0)
	default:
		logInfo("未知命令: %s, 输入 'help' 查看帮助", cmd)
	}
}

func printHelp() {
	fmt.Println(`
可用命令:
  status    - 显示节点状态
  gvs       - 显示游戏价值评分
  games     - 显示游戏数据
  stats     - 显示网络统计
  set       - 设置配置
  help      - 显示帮助
  exit      - 退出节点
`)
}

func printStatus() {
	fmt.Printf(`
节点信息:
  ID:          %s
  名称:        %s
  状态:        %s
  质押:        %d POLE
  信誉:        %.2f
  在线时间:    %s
  区块高度:    %d
  Epoch:       %d
`, node.ID, node.Name, node.Status, node.Stake, node.Reputation, 
   time.Since(startTime).String(), networkStats.Height, networkStats.Epoch)
}

func printGVS() {
	fmt.Println("\n游戏价值评分 (GVS):")
	fmt.Println("----------------------------------------")
	fmt.Printf("%-12s %-12s %-8s %s\n", "游戏ID", "GVS评分", "等级", "更新时间")
	fmt.Println("----------------------------------------")
	
	for _, g := range gvsMap {
		tierName := []string{"", "Tier1", "Tier2", "Tier3"}
		tier := "Unknown"
		if g.Tier >= 1 && g.Tier <= 3 {
			tier = tierName[g.Tier]
		}
		fmt.Printf("%-12s %-12.2f %-8s %s\n", 
			g.GameID, g.Score, tier, 
			time.Unix(g.Updated, 0).Format("15:04:05"))
	}
}

func printGames() {
	fmt.Println("\n游戏数据:")
	fmt.Println("----------------------------------------")
	
	for id, data := range gameData {
		if len(data) > 0 {
			latest := data[len(data)-1]
			fmt.Printf("%s: %d 条记录, 最新在线: %d\n", 
				id, len(data), latest.OnlinePlayers)
		}
	}
}

func printStats() {
	fmt.Printf(`
网络统计:
  区块高度:     %d
  Epoch:       %d
  总节点:      %d
  活跃节点:    %d
  总GVS:       %.2f
  运行时间:    %s
`, networkStats.Height, networkStats.Epoch, 
   networkStats.TotalNodes, networkStats.ActiveNodes,
   networkStats.TotalGVS, time.Since(startTime).String())
}

func setConfig(key, value string) {
	switch key {
	case "name":
		config.NodeName = value
		node.Name = value
		logInfo("节点名称已设置为: %s", value)
	case "api_port":
		if port, err := strconv.Atoi(value); err == nil {
			config.APIPort = port
			logInfo("API端口已设置为: %d", port)
		} else {
			logError("无效的端口号")
		}
	default:
		logError("未知配置项: %s", key)
	}
}

// ==================== 信号处理 ====================

func waitForSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	<-sigChan
	logInfo("\n收到停止信号，正在关闭节点...")
	stopChan <- true
	time.Sleep(1 * time.Second)
	logInfo("节点已安全关闭")
}

// ==================== 主程序 ====================

func main() {
	// 解析命令行参数
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		
		switch arg {
		case "--name", "-n":
			if i+1 < len(os.Args) {
				config.NodeName = os.Args[i+1]
				i++
			}
		case "--port", "-p":
			if i+1 < len(os.Args) {
				if port, err := strconv.Atoi(os.Args[i+1]); err == nil {
					config.APIPort = port
					i++
				}
			}
		case "--api":
			config.APIEnabled = true
		case "--no-api":
			config.APIEnabled = false
		case "--help", "-h":
			fmt.Println(`
PoLE 节点程序

用法: pole-node [选项]

选项:
  -n, --name <名称>    设置节点名称
  -p, --port <端口>    设置API端口 (默认 8080)
  --api                启用API服务器 (默认启用)
  --no-api             禁用API服务器
  -h, --help           显示帮助

交互命令:
  status   - 显示节点状态
  gvs      - 显示游戏价值评分
  games    - 显示游戏数据
  stats    - 显示网络统计
  exit     - 退出节点
`)
			return
		}
	}
	
	node.Name = config.NodeName
	
	// 启动节点
	go startNode()
	
	// 启动交互模式
	startInteractive()
}
