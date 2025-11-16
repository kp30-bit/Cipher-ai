package usecase

import (
	"concall-analyser/config"
	"concall-analyser/internal/db"
	"concall-analyser/internal/domain"
	"concall-analyser/internal/infrastructure/http"
	"concall-analyser/internal/interfaces"
	"concall-analyser/internal/repository/mongo"
	"concall-analyser/internal/service/analytics"
	"concall-analyser/internal/service/bse"
	"concall-analyser/internal/service/pdf"
)

type concallFetcher struct {
	repo             domain.ConcallRepository
	bseClient        bse.BSEClient
	pdfDownloader    pdf.PDFDownloader
	analyticsService analytics.AnalyticsService
	cfg              *config.Config
}

// NewConcallFetcher creates a new usecase instance with dependency injection
func NewConcallFetcher(db *db.MongoDB, cfg *config.Config, analyticsService analytics.AnalyticsService) (interfaces.Usecase, error) {
	repo := mongo.NewConcallRepository(db)
	httpClient := http.NewHTTPClient()
	bseClient := bse.NewBSEClient(httpClient)
	pdfDownloader := pdf.NewPDFDownloader(httpClient)

	return &concallFetcher{
		repo:             repo,
		bseClient:        bseClient,
		pdfDownloader:    pdfDownloader,
		analyticsService: analyticsService,
		cfg:              cfg,
	}, nil
}
