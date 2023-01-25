package converter

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"moredoc/util"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	soffice      = "soffice"
	ebookConvert = "ebook-convert"
	svgo         = "svgo"
	mutool       = "mutool"
	dirDteFmt    = "2006/01/02"
)

type ConvertCallback func(page int, pagePath string, err error)

type Converter struct {
	cachePath string
	timeout   time.Duration
	logger    *zap.Logger
	workspace string
}

type Page struct {
	PageNum  int
	PagePath string
}

// NewConverter 创建一个新的转换器。每个不同的原始文档，都需要一个新的转换器。
// 因为最后可以清空该原始文档及其衍生文件的临时目录，用节省磁盘空间。
func NewConverter(logger *zap.Logger, timeout ...time.Duration) *Converter {
	expire := 1 * time.Hour
	if len(timeout) > 0 {
		expire = timeout[0]
	}
	defaultCachePath := "cache/convert"
	os.MkdirAll(defaultCachePath, os.ModePerm)
	return &Converter{
		cachePath: defaultCachePath,
		timeout:   expire,
		logger:    logger.Named("converter"),
	}
}

func (c *Converter) SetCachePath(cachePath string) {
	os.MkdirAll(cachePath, os.ModePerm)
	c.cachePath = cachePath
}

// ConvertToPDF 将文件转为PDF。
// 自动根据文件类型调用相应的转换函数。
func (c *Converter) ConvertToPDF(src string) (dst string, err error) {
	ext := strings.ToLower(filepath.Ext(src))
	switch ext {
	case ".epub":
		return c.ConvertEPUBToPDF(src)
	case ".umd":
		return c.ConvertUMDToPDF(src)
	case ".txt":
		return c.ConvertTXTToPDF(src)
	case ".mobi":
		return c.ConvertMOBIToPDF(src)
	case ".chm":
		return c.ConvertCHMToPDF(src)
	case ".pdf":
		return c.PDFToPDF(src)
	// case ".doc", ".docx", ".rtf", ".wps", ".odt",
	// 	".xls", ".xlsx", ".et", ".ods",
	// 	".ppt", ".pptx", ".dps", ".odp", ".pps", ".ppsx", ".pot", ".potx":
	// 	return c.ConvertOfficeToPDF(src)
	default:
		return c.ConvertOfficeToPDF(src)
		// return "", fmt.Errorf("不支持的文件类型：%s", ext)
	}
}

// ConvertOfficeToPDF 通过soffice将office文档转换为pdf
func (c *Converter) ConvertOfficeToPDF(src string) (dst string, err error) {
	return c.convertToPDFBySoffice(src)
}

// ConvertEPUBToPDF 将 epub 转为PDF
func (c *Converter) ConvertEPUBToPDF(src string) (dst string, err error) {
	return c.convertToPDFByCalibre(src)
}

// ConvertUMDToPDF 将umd转为PDF
func (c *Converter) ConvertUMDToPDF(src string) (dst string, err error) {
	return c.convertToPDFBySoffice(src)
}

// ConvertTXTToPDF 将 txt 转为PDF
func (c *Converter) ConvertTXTToPDF(src string) (dst string, err error) {
	return c.convertToPDFBySoffice(src)
}

// ConvertMOBIToPDF 将 mobi 转为PDF
func (c *Converter) ConvertMOBIToPDF(src string) (dst string, err error) {
	return c.convertToPDFByCalibre(src)
}

// ConvertPDFToTxt 将PDF转为TXT
func (c *Converter) ConvertPDFToTxt(src string) (dst string, err error) {
	workspace := c.makeWorkspace(src)
	dst = workspace + "/dst.txt"
	args := []string{
		"convert",
		"-o",
		dst,
		src,
	}
	c.logger.Debug("convert pdf to txt", zap.String("cmd", mutool), zap.Strings("args", args))
	_, err = util.ExecCommand(mutool, args, c.timeout)
	if err != nil {
		c.logger.Error("convert pdf to txt", zap.String("cmd", mutool), zap.Strings("args", args), zap.Error(err))
		return
	}
	return dst, nil
}

// ConvertCHMToPDF 将CHM转为PDF
func (c *Converter) ConvertCHMToPDF(src string) (dst string, err error) {
	return c.convertToPDFByCalibre(src)
}

// ConvertPDFToSVG 将PDF转为SVG
func (c *Converter) ConvertPDFToSVG(src string, fromPage, toPage int, enableSVGO, enableGZIP bool) (pages []Page, err error) {
	pages, err = c.convertPDFToPage(src, fromPage, toPage, ".svg")
	if err != nil {
		return
	}

	if len(pages) == 0 {
		return
	}

	if enableSVGO { // 压缩svg
		c.CompressSVGBySVGO(filepath.Dir(pages[0].PagePath))
	}

	if enableGZIP { // gzip 压缩
		for idx, page := range pages {
			if dst, errCompress := c.CompressSVGByGZIP(page.PagePath); errCompress == nil {
				os.Remove(page.PagePath)
				page.PagePath = dst
				pages[idx] = page
			}
		}
	}
	return
}

// ConvertPDFToPNG 将PDF转为PNG
func (c *Converter) ConvertPDFToPNG(src string, fromPage, toPage int) (pages []Page, err error) {
	// return c.convertPDFToPage(src, fromPage, toPage, ".png")
	pages = nil
	err = nil
	return
}

func (c *Converter) PDFToPDF(src string) (dst string, err error) {
	workspace := c.makeWorkspace(src)
	dst = workspace + "/dst.pdf"
	err = util.CopyFile(src, dst)
	if err != nil {
		c.logger.Error("copy file error", zap.Error(err))
	}
	return
}

// ext 可选值： .png, .svg
func (c *Converter) convertPDFToPage(src string, fromPage, toPage int, ext string) (pages []Page, err error) {
	pageRange := fmt.Sprintf("%d-%d", fromPage, toPage)
	workspace := c.makeWorkspace(src)
	cacheFileFormat := workspace + "/%d" + ext

	args := []string{
		"convert",
		"-o",
		cacheFileFormat,
		src,
		pageRange,
	}

	args1 := []string{
		src,
		cacheFileFormat,
		"all",
	}

	c.logger.Debug("convert pdf to page", zap.String("cmd", mutool), zap.Strings("args", args))
	cmd := exec.Command("pdf2svg", args1...)
	err = cmd.Run()
	// _, err = util.ExecCommand(mutool, args, c.timeout)
	if err != nil {
		return
	}

	for i := 0; i <= toPage-fromPage; i++ {
		pagePath := fmt.Sprintf(cacheFileFormat, i+1)
		if _, errPage := os.Stat(pagePath); errPage != nil {
			break
		}
		pages = append(pages, Page{
			PageNum:  fromPage + i,
			PagePath: pagePath,
		})
	}
	return
}

// 将 PDF 转成 SVG
// func ConvertPDF2SVG(pdfFile, svgFile string, pageNO int) (err error) {
// 	pdf2svg := strings.TrimSpace(GetConfig("depend", "pdf2svg", "pdf2svg"))

// 	//Usage: pdf2svg <in file.pdf> <out file.svg> [<page no>]
// 	pdfFile, _ = filepath.Abs(pdfFile)
// 	svgFile, _ = filepath.Abs(svgFile)
// 	args := []string{pdfFile, svgFile, strconv.Itoa(pageNO)}
// 	cmd := exec.Command(pdf2svg, args...)
// 	if strings.HasPrefix(pdf2svg, "sudo") {
// 		args = append([]string{strings.TrimPrefix(pdf2svg, "sudo")}, args...)
// 		cmd = exec.Command("sudo", args...)
// 	}
// 	time.AfterFunc(30*time.Second, func() {
// 		if cmd.Process != nil {
// 			cmd.Process.Kill()
// 		}
// 	})

// 	err = cmd.Run()
// 	return
// }

func (c *Converter) convertToPDFBySoffice(src string) (dst string, err error) {
	workspace := c.makeWorkspace(src)
	dst = workspace + "/" + strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)) + ".pdf"
	args := []string{
		"--headless",
		"--convert-to",
		"pdf",
		"--outdir",
		workspace,
	}
	args = append(args, src)
	c.logger.Debug("convert to pdf by soffice", zap.String("cmd", soffice), zap.Strings("args", args))
	_, err = util.ExecCommand(soffice, args, c.timeout)
	if err != nil {
		c.logger.Error("convert to pdf by soffice", zap.String("cmd", soffice), zap.Strings("args", args), zap.Error(err))
	}
	return
}

func (c *Converter) convertToPDFByCalibre(src string) (dst string, err error) {
	workspace := c.makeWorkspace(src)
	dst = workspace + "/dst.pdf"
	args := []string{
		src,
		dst,
		"--paper-size", "a4",
		// "--pdf-default-font-size", "14",
		"--pdf-page-margin-bottom", "36",
		"--pdf-page-margin-left", "36",
		"--pdf-page-margin-right", "36",
		"--pdf-page-margin-top", "36",
	}
	os.MkdirAll(filepath.Dir(dst), os.ModePerm)
	c.logger.Debug("convert to pdf by calibre", zap.String("cmd", ebookConvert), zap.Strings("args", args))
	_, err = util.ExecCommand(ebookConvert, args, c.timeout)
	if err != nil {
		c.logger.Error("convert to pdf by calibre", zap.String("cmd", ebookConvert), zap.Strings("args", args), zap.Error(err))
	}
	return
}

func (c *Converter) CountPDFPages(file string) (pages int, err error) {
	args := []string{
		"show",
		file,
		"pages",
	}
	c.logger.Debug("count pdf pages", zap.String("cmd", mutool), zap.Strings("args", args))
	var out string
	out, err = util.ExecCommand(mutool, args, c.timeout)
	if err != nil {
		c.logger.Error("count pdf pages", zap.String("cmd", mutool), zap.Strings("args", args), zap.Error(err))
		return
	}

	lines := strings.Split(out, "\n")
	length := len(lines)
	for i := length - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.ToLower(lines[i]))
		c.logger.Debug("count pdf pages", zap.String("line", line))
		if strings.HasPrefix(line, "page") {
			pages, _ = strconv.Atoi(strings.TrimSpace(strings.TrimLeft(strings.Split(line, "=")[0], "page")))
			if pages > 0 {
				return
			}
		}
	}
	return
}

func (c *Converter) ExistMupdf() (err error) {
	_, err = exec.LookPath(mutool)
	return
}

func (c *Converter) ExistSoffice() (err error) {
	_, err = exec.LookPath(soffice)
	return
}

func (c *Converter) ExistCalibre() (err error) {
	_, err = exec.LookPath(ebookConvert)
	return
}

func (c *Converter) ExistSVGO() (err error) {
	_, err = exec.LookPath(svgo)
	return
}

func (c *Converter) CompressSVGBySVGO(svgFolder string) (err error) {
	args := []string{
		"-f",
		svgFolder,
	}
	c.logger.Debug("compress svg by svgo", zap.String("cmd", svgo), zap.Strings("args", args))
	var out string
	out, err = util.ExecCommand(svgo, args, c.timeout*10)
	if err != nil {
		c.logger.Error("compress svg by svgo", zap.String("cmd", svgo), zap.Strings("args", args), zap.Error(err))
	}
	c.logger.Debug("compress svg by svgo", zap.String("out", out))
	return
}

// CompressSVGByGZIP 将SVG文件压缩为GZIP格式
func (c *Converter) CompressSVGByGZIP(svgFile string) (dst string, err error) {
	var svgBytes []byte
	dst = strings.TrimSuffix(svgFile, filepath.Ext(svgFile)) + ".gzip.svg"
	svgBytes, err = os.ReadFile(svgFile)
	if err != nil {
		c.logger.Error("read svg file", zap.String("svgFile", svgFile), zap.Error(err))
		return
	}

	var replaces = map[string]string{
		`data-text="<"`: `data-text="&lt;"`,
		`data-text=">"`: `data-text="&gt;"`,
	}
	for k, v := range replaces {
		svgBytes = bytes.ReplaceAll(svgBytes, []byte(k), []byte(v))
	}

	var buf bytes.Buffer
	gzw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	defer gzw.Close()
	gzw.Write(svgBytes)
	gzw.Flush()
	err = os.WriteFile(dst, buf.Bytes(), os.ModePerm)
	if err != nil {
		c.logger.Error("write svgz file", zap.String("svgzFile", dst), zap.Error(err))
	}
	return
}

func (c *Converter) makeWorkspace(src string) (workspaceDir string) {
	if c.workspace != "" {
		workspaceDir = c.workspace
		return
	}
	c.workspace = strings.ReplaceAll(filepath.Join(c.cachePath, time.Now().Format(dirDteFmt), strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))), "\\", "/")
	return c.workspace
}

func (c *Converter) Clean() (err error) {
	if c.workspace != "" {
		c.logger.Info("clean workspace", zap.String("workspace", c.workspace))
		err = os.RemoveAll(c.workspace)
		if err != nil {
			c.logger.Error("clean workspace", zap.String("workspace", c.workspace), zap.Error(err))
		} else {
			c.logger.Info("clean workspace success", zap.String("workspace", c.workspace))
		}
		c.workspace = ""
	}
	return
}
