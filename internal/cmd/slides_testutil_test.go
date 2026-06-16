package cmd

import (
	"context"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/app"
)

func stubSlidesService(svc *slides.Service) app.SlidesServiceFactory {
	return func(context.Context, string) (*slides.Service, error) { return svc, nil }
}

func withSlidesTestService(ctx context.Context, svc *slides.Service) context.Context {
	return withSlidesTestServiceFactory(ctx, stubSlidesService(svc))
}

func withSlidesTestServiceFactory(ctx context.Context, factory app.SlidesServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Slides = factory
	return app.WithRuntime(ctx, runtime)
}

func withSlidesAndDriveTestServices(ctx context.Context, slidesSvc *slides.Service, driveSvc *drive.Service) context.Context {
	return withSlidesTestService(withDriveTestService(ctx, driveSvc), slidesSvc)
}
