package m3u8dcpp

import (
	"context"
	"github.com/orestonce/m3u8d"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var gMergeStatus m3u8d.SpeedStatus
var gMergeCancel context.CancelFunc
var gMergeCancelLocker sync.Mutex

type MergeTsDir_Resp struct {
	ErrMsg   string
	IsCancel bool
}

func beginMerge() bool {
	gMergeStatus.Locker.Lock()
	defer gMergeStatus.Locker.Unlock()

	if gMergeStatus.IsRunning != false {
		return false
	}
	gMergeStatus.IsRunning = true
	return true
}

func MergeTsDir(InputTsDir string, OutputMp4Name string, UseFirstTsMTime bool, SkipBadResolutionFps bool) (resp MergeTsDir_Resp) {
	if !beginMerge() {
		return resp
	}

	defer func() {
		gMergeStatus.Locker.Lock()
		gMergeStatus.IsRunning = false
		gMergeStatus.Locker.Unlock()
	}()

	fList, err := ioutil.ReadDir(InputTsDir)
	if err != nil {
		resp.ErrMsg = "读取目录失败 " + err.Error()
		return
	}
	var tsFileList []string
	for _, f := range fList {
		if f.Mode().IsRegular() && strings.HasSuffix(strings.ToLower(f.Name()), ".ts") {
			tsFileList = append(tsFileList, filepath.Join(InputTsDir, f.Name()))
		}
	}
	if len(tsFileList) == 0 {
		resp.ErrMsg = "目录下不存在ts文件: " + InputTsDir
		return
	}
	sort.Strings(tsFileList) // 按照字典顺序排序
	if OutputMp4Name == "" {
		OutputMp4Name = filepath.Join(InputTsDir, "all.mp4")
	} else if !filepath.IsAbs(OutputMp4Name) {
		OutputMp4Name = filepath.Join(InputTsDir, OutputMp4Name)
	}
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	gMergeCancelLocker.Lock()
	if gMergeCancel != nil {
		gMergeCancel()
	}
	gMergeCancel = cancelFn
	gMergeCancelLocker.Unlock()

	if SkipBadResolutionFps {
		tsFileList, err = m3u8d.AnalyzeTs(&gMergeStatus, tsFileList, OutputMp4Name, ctx)
		if err != nil {
			resp.ErrMsg = "分析ts失败: " + err.Error()
			resp.IsCancel = m3u8d.IsContextCancel(ctx)
			return
		}
	}
	gMergeStatus.SetProgressBarTitle("合并ts")
	gMergeStatus.SpeedResetTotalBlockCount(len(tsFileList))

	err = m3u8d.MergeTsFileListToSingleMp4(m3u8d.MergeTsFileListToSingleMp4_Req{
		TsFileList: tsFileList,
		OutputMp4:  OutputMp4Name,
		Ctx:        ctx,
		Status:     &gMergeStatus,
	})
	if err == nil && UseFirstTsMTime {
		err = m3u8d.UpdateMp4Time(tsFileList[0], OutputMp4Name)
	}
	if err != nil {
		resp.ErrMsg = "合并错误: " + err.Error()
		resp.IsCancel = m3u8d.IsContextCancel(ctx)
		return resp
	}
	return resp
}

type MergeGetProgressPercent_Resp struct {
	Title     string
	Percent   int
	SpeedText string
	IsRunning bool
}

func MergeGetProgressPercent() (resp MergeGetProgressPercent_Resp) {
	gMergeStatus.Locker.Lock()
	resp.IsRunning = gMergeStatus.IsRunning
	gMergeStatus.Locker.Unlock()

	if resp.IsRunning {
		resp.Percent = gMergeStatus.GetPercent()
		resp.SpeedText = gMergeStatus.SpeedRecent5sGetAndUpdate().ToString()
		resp.Title = gMergeStatus.GetTitle()
	} else {
		resp.Title = "进度"
	}
	return resp
}

func MergeStop() {
	gMergeCancelLocker.Lock()
	if gMergeCancel != nil {
		gMergeCancel()
	}
	gMergeCancelLocker.Unlock()
}
