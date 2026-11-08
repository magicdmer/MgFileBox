package service

import (
	"context"
	"errors"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"

	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
	"mgfilebox/internal/security"
)

func (s *Service) ReplaceShareFile(ctx context.Context, shareID, fileID string, header *multipart.FileHeader) error {
	if header == nil {
		return errors.New("请选择一个新文件")
	}
	if s.cfg.MaxUploadSize > 0 && header.Size > s.cfg.MaxUploadSize {
		return ErrUploadTooLarge
	}
	share, err := s.repo.GetShareByID(ctx, shareID)
	if err != nil {
		return err
	}
	if share.IsDeleted() {
		return repository.ErrNotFound
	}
	old, err := s.repo.GetShareFile(ctx, shareID, fileID)
	if err != nil {
		return err
	}
	token, err := security.RandomToken(16)
	if err != nil {
		return err
	}
	file := *old
	file.FileName = filepath.Base(header.Filename)
	file.MimeType = header.Header.Get("Content-Type")
	file.Size = header.Size
	file.StoragePath = filepath.Join(s.cfg.UploadDir, shareID+"-"+token+filepath.Ext(file.FileName))
	if err := saveUploadedFile(header, file.StoragePath); err != nil {
		return err
	}
	// Upload outside the lock; serialize only the metadata switch with deletion
	// and opening downloads. The old path also acts as a concurrent-update guard.
	s.fileMu.Lock()
	err = s.repo.ReplaceShareFile(ctx, *old, file)
	s.fileMu.Unlock()
	if err != nil {
		return errors.Join(err, removeFiles([]string{file.StoragePath}))
	}
	if err := s.cleanupObsoleteFiles(context.WithoutCancel(ctx)); err != nil {
		log.Printf("清理旧版文件失败，将在后续清理时重试：%v", err)
	}
	return nil
}

// OpenShareFile pins an opened version before a replacement can retire its path.
func (s *Service) OpenShareFile(ctx context.Context, shareID, fileID string) (*os.File, *models.ShareFile, error) {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	share, err := s.repo.GetShareByID(ctx, shareID)
	if err != nil {
		return nil, nil, err
	}
	if share.IsDeleted() {
		return nil, nil, repository.ErrNotFound
	}
	var target *models.ShareFile
	for index := range share.Files {
		if share.Files[index].ID == fileID || fileID == "" {
			target = &share.Files[index]
			break
		}
	}
	if target == nil {
		return nil, nil, repository.ErrNotFound
	}
	file, err := os.Open(target.StoragePath)
	return file, target, err
}

func (s *Service) cleanupObsoleteFiles(ctx context.Context) error {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	paths, err := s.repo.ListObsoleteFiles(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
			continue
		}
		failures = append(failures, s.repo.ForgetObsoleteFile(ctx, path))
	}
	return errors.Join(failures...)
}
