package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

// 代码块结构
type Chunk struct {
	StartLine int    // 起始行号
	EndLine   int    // 结束行号
	Content   string // 代码内容
}

// 代码转PDF配置
type Config struct {
	InputPath    string  // 输入文件/目录路径
	OutputPath   string  // 输出PDF路径
	ProjectName  string  // 项目名称（用于页眉）
	LinesPerPage int     // 每页行数
	TotalPages   int     // 总页数
	CodeChunks   []Chunk // 代码块（前30页和后30页）
}

func main() {
	// 解析命令行参数
	config := parseFlags()

	// 判断是文件还是目录
	fileInfo, err := os.Stat(config.InputPath)
	if err != nil {
		log.Fatalf("获取文件信息失败: %v", err)
	}

	if fileInfo.IsDir() {
		// 处理目录 - 将所有代码文件合并为一个PDF
		err := processDirectory(&config)
		if err != nil {
			log.Fatalf("处理目录失败: %v", err)
		}
	} else {
		// 处理单个文件
		err := processCodeFiles(&config)
		if err != nil {
			log.Fatalf("处理代码文件失败: %v", err)
		}

		// 生成PDF
		err = generatePDF(&config)
		if err != nil {
			log.Fatalf("生成PDF失败: %v", err)
		}

		fmt.Printf("PDF生成成功: %s\n", config.OutputPath)
	}
}

// 处理目录中的所有代码文件，合并为一个PDF
func processDirectory(config *Config) error {
	// 创建输出目录（如果不存在）
	outputDir := filepath.Dir(config.OutputPath)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		err := os.MkdirAll(outputDir, 0755)
		if err != nil {
			return fmt.Errorf("创建输出目录失败: %v", err)
		}
	}

	// 收集所有代码文件
	var codeFiles []string
	err := filepath.Walk(config.InputPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 只处理代码文件
		if isCodeFile(info.Name()) {
			codeFiles = append(codeFiles, path)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("遍历目录失败: %v", err)
	}

	// 按文件名排序
	sort.Strings(codeFiles)

	// 创建PDF对象
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 10) // 减小页脚边距，使内容更紧凑

	// 设置总页数占位符
	pdf.AliasNbPages("{nb}")

	// 添加中文字体支持
	pdf.AddUTF8Font("msyh", "", "fonts/微软雅黑.ttf")

	// 使用Courier字体处理代码（等宽字体，更适合代码显示）
	pdf.SetFont("Courier", "", 8)

	// 设置字符间距
	pdf.SetCellMargin(0)

	// 添加页眉和页脚
	pdf.SetHeaderFunc(func() {
		pdf.SetY(5) // 减小页眉顶部间距，将页眉上移
		pdf.SetFont("msyh", "", 10)
		pdf.CellFormat(0, 8, fmt.Sprintf("%s ", config.ProjectName), "", 0, "L", false, 0, "")
		pdf.Ln(8) // 减小页眉底部间距
		// 添加分割线
		pdf.SetLineWidth(0.5)
		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
		pdf.Ln(3) // 减小分割线后间距
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)                                                                                // 减小页脚底部边距
		pdf.SetFont("msyh", "", 8)                                                                   // 使用微软雅黑显示中文
		pdf.CellFormat(0, 8, fmt.Sprintf("第 %d 页，共 {nb} 页", pdf.PageNo()), "", 0, "C", false, 0, "") // 使用占位符表示总页数
		pdf.Ln(4)                                                                                    // 减小页脚内部间距
	})

	// 添加第一页
	pdf.AddPage()

	// 处理每个文件
	for _, filePath := range codeFiles {
		// 读取文件内容
		lines, err := readFileLines(filePath)
		if err != nil {
			return fmt.Errorf("读取文件 %s 失败: %v", filePath, err)
		}

		// 文件之间不添加任何分隔或标题，直接连接

		// 按每页行数分页显示代码
		linesPerPage := config.LinesPerPage

		// 显示全部代码，不再分割长文件
		displayCodeLines(pdf, lines, 1, linesPerPage)
	}

	// 输出PDF
	err = pdf.OutputFileAndClose(config.OutputPath)
	if err != nil {
		return fmt.Errorf("生成PDF失败: %v", err)
	}

	fmt.Printf("PDF生成成功: %s (包含 %d 个代码文件)\n", config.OutputPath, len(codeFiles))
	return nil
}

// 显示代码行
func displayCodeLines(pdf *gofpdf.Fpdf, lines []string, startLineNumber int, linesPerPage int) {
	totalLines := len(lines)

	// 获取当前页面剩余空间
	currentY := pdf.GetY()
	pageHeight := float64(297 - 20) // A4纸高度减去页边距，转换为float64
	remainingSpace := pageHeight - currentY

	// 估算每行代码的高度
	lineHeight := 5.0 // 每行代码的估计高度

	for i := 0; i < totalLines; i += linesPerPage {
		// 计算当前批次代码需要的空间
		batchLines := linesPerPage
		if i+batchLines > totalLines {
			batchLines = totalLines - i
		}

		// 估算当前批次代码需要的空间
		neededSpace := float64(batchLines) * lineHeight

		// 如果当前页面剩余空间不足，添加新页
		if neededSpace > remainingSpace {
			pdf.AddPage()
			remainingSpace = pageHeight // 重置剩余空间为页面高度
		}

		// 每页显示指定行数
		end := i + linesPerPage
		if end > totalLines {
			end = totalLines
		}

		pageLines := lines[i:end]

		// 每页的行号从1开始
		pageLineNumber := 1

		// 创建代码表格
		for _, line := range pageLines {
			// 行号宽度
			lineNumWidth := float64(15) // 增加行号宽度，为行号和代码之间创造更多空间

			// 行号单元格
			pdf.SetTextColor(128, 128, 128)                                                               // 灰色
			pdf.CellFormat(lineNumWidth, 5, fmt.Sprintf("%4d", pageLineNumber), "", 0, "R", false, 0, "") // 每页从1开始
			pageLineNumber++                                                                              // 页内行号递增

			// 添加间距单元格
			spacerWidth := float64(5)                                    // 额外的间距宽度
			pdf.CellFormat(spacerWidth, 5, "", "", 0, "L", false, 0, "") // 空白间距单元格

			// 代码单元格
			pdf.SetTextColor(0, 0, 0)  // 黑色
			pdf.SetFont("msyh", "", 8) // 使用微软雅黑字体显示代码内容，支持中文
			availableWidth := float64(190) - lineNumWidth - spacerWidth
			pdf.MultiCell(availableWidth, 5, line, "", "L", false) // 左对齐代码内容
		}

		// 表格底部横线 - 减小间距
		pdf.Ln(0.5)
		pdf.SetLineWidth(0.2) // 减小线宽
		pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
		pdf.Ln(2) // 减小底部间距

		// 更新剩余空间
		currentY = pdf.GetY()
		remainingSpace = pageHeight - currentY // 现在pageHeight已经是float64类型，这里不会有类型不匹配的问题
	}
}

// 解析命令行参数
func parseFlags() Config {
	inputPath := flag.String("input", "", "输入文件或目录路径")
	outputPath := flag.String("output", "code_document.pdf", "输出PDF路径")
	projectName := flag.String("project", "项目名称", "项目名称（用于页眉）")
	linesPerPage := flag.Int("lines-per-page", 50, "每页行数")
	totalPages := flag.Int("total-pages", 60, "总页数")

	flag.Parse()

	if *inputPath == "" {
		log.Fatal("请指定输入文件或目录路径 (-input)")
	}

	return Config{
		InputPath:    *inputPath,
		OutputPath:   *outputPath,
		ProjectName:  *projectName,
		LinesPerPage: *linesPerPage,
		TotalPages:   *totalPages,
	}
}

// 处理代码文件
func processCodeFiles(config *Config) error {
	var allCodeLines []string

	// 处理单个文件
	lines, err := readFileLines(config.InputPath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	allCodeLines = lines

	// 计算需要提取的代码块
	totalLines := len(allCodeLines)

	// 如果总行数超过配置的页数限制，取前一半和后一半
	if totalLines > config.LinesPerPage*config.TotalPages {
		halfPages := config.LinesPerPage * config.TotalPages / 2
		firstChunk := Chunk{
			StartLine: 1,
			EndLine:   halfPages,
			Content:   strings.Join(allCodeLines[:halfPages], "\n"),
		}

		lastChunkStart := totalLines - halfPages
		lastChunk := Chunk{
			StartLine: lastChunkStart + 1,
			EndLine:   totalLines,
			Content:   strings.Join(allCodeLines[lastChunkStart:], "\n"),
		}

		config.CodeChunks = []Chunk{firstChunk, lastChunk}
	} else {
		// 如果总行数不足配置的页数限制，取全部代码
		config.CodeChunks = []Chunk{
			{
				StartLine: 1,
				EndLine:   totalLines,
				Content:   strings.Join(allCodeLines, "\n"),
			},
		}
	}

	return nil
}

// 判断是否为代码文件
func isCodeFile(filename string) bool {
	codeExtensions := []string{
		".go", ".java", ".py", ".js", ".ts", ".html", ".css", ".cpp", ".c", ".h", ".cs", ".php",
	}

	ext := filepath.Ext(filename)
	for _, e := range codeExtensions {
		if e == ext {
			return true
		}
	}

	return false
}

// 读取文件行
func readFileLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败 %s: %w", path, err)
	}

	// 处理不同的换行符
	text := string(data)
	text = strings.ReplaceAll(text, "\r\n", "\n") // Windows换行符转换为Unix换行符
	text = strings.ReplaceAll(text, "\r", "\n")   // 旧Mac换行符转换为Unix换行符

	// 处理特殊空格字符，防止在PDF中显示为方框
	text = strings.ReplaceAll(text, "\t", "    ") // 将制表符替换为4个空格

	// 处理其他可能导致方框显示的特殊空格字符
	text = strings.Map(func(r rune) rune {
		// 如果是不可见字符但不是普通空格、换行符或回车符，则替换为普通空格
		if (r < 32 || r == 127 || (r >= 128 && r <= 159)) && r != '\n' && r != '\r' && r != ' ' {
			return ' '
		}
		return r
	}, text)

	return strings.Split(text, "\n"), nil
}

// 生成PDF
func generatePDF(config *Config) error {
	// 创建PDF对象 - 使用中文支持
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 10) // 减小页脚边距，使内容更紧凑

	// 设置总页数占位符
	pdf.AliasNbPages("{nb}")

	// 添加中文字体支持
	pdf.AddUTF8Font("msyh", "", "fonts/微软雅黑.ttf")

	// 使用Courier字体处理代码（等宽字体，更适合代码显示）
	pdf.SetFont("Courier", "", 8)

	// 设置字符间距
	pdf.SetCellMargin(0)

	// 添加页眉和页脚
	pdf.SetHeaderFunc(func() {
		pdf.SetY(5) // 减小页眉顶部间距，将页眉上移
		pdf.SetFont("msyh", "", 10)
		pdf.CellFormat(0, 8, fmt.Sprintf("%s ", config.ProjectName), "", 0, "L", false, 0, "")
		pdf.Ln(8) // 减小页眉底部间距
		// 添加分割线
		pdf.SetLineWidth(0.5)
		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
		pdf.Ln(3) // 减小分割线后间距
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)                                                                                // 减小页脚底部边距
		pdf.SetFont("msyh", "", 8)                                                                   // 使用微软雅黑显示中文
		pdf.CellFormat(0, 8, fmt.Sprintf("第 %d 页，共 {nb} 页", pdf.PageNo()), "", 0, "C", false, 0, "") // 使用占位符表示总页数
		pdf.Ln(4)                                                                                    // 减小页脚内部间距
	})

	// 添加第一页
	pdf.AddPage()

	// 处理每个代码块，不添加标题页，直接显示代码内容
	for _, chunk := range config.CodeChunks {

		// 分割代码内容为行
		lines := strings.Split(chunk.Content, "\n")

		// 按每页行数分页显示代码
		linesPerPage := config.LinesPerPage

		for i := 0; i < len(lines); i += linesPerPage {
			// 如果不是第一页，添加新页
			if i > 0 {
				pdf.AddPage()
			}

			// 每页显示指定行数
			end := i + linesPerPage
			if end > len(lines) {
				end = len(lines)
			}

			pageLines := lines[i:end]

			// 每页重置行号计数器
			pageLineNumber := 1

			// 创建代码表格
			for _, line := range pageLines {
				// 行号宽度
				lineNumWidth := float64(15) // 增加行号宽度，为行号和代码之间创造更多空间

				// 行号单元格
				pdf.SetTextColor(128, 128, 128)                                                               // 灰色
				pdf.CellFormat(lineNumWidth, 5, fmt.Sprintf("%4d", pageLineNumber), "", 0, "R", false, 0, "") // 右对齐行号，每页从1开始

				// 添加间距单元格
				spacerWidth := float64(5)                                    // 额外的间距宽度
				pdf.CellFormat(spacerWidth, 5, "", "", 0, "L", false, 0, "") // 空白间距单元格

				// 代码单元格
				pdf.SetTextColor(0, 0, 0)  // 黑色
				pdf.SetFont("msyh", "", 8) // 使用微软雅黑字体显示代码内容，支持中文
				availableWidth := float64(190) - lineNumWidth - spacerWidth
				pdf.MultiCell(availableWidth, 5, line, "", "L", false) // 左对齐代码内容

				pageLineNumber++ // 页内行号递增
			}

			// 表格底部横线 - 减小间距
			pdf.Ln(0.5)
			pdf.SetLineWidth(0.2) // 减小线宽
			pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
			pdf.Ln(2) // 减小底部间距
		}
	}

	// 输出PDF
	return pdf.OutputFileAndClose(config.OutputPath)
}
