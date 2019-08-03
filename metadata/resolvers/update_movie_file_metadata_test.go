package resolvers

import (
	"context"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"gitlab.com/olaris/olaris-server/metadata/agents/agentsfakes"
	"gitlab.com/olaris/olaris-server/metadata/app"
	"gitlab.com/olaris/olaris-server/metadata/auth"
	"gitlab.com/olaris/olaris-server/metadata/db"
	"testing"
)

func TestUpdateMovieFile(t *testing.T) {
	const testTmdbID = 1234
	const testUserID = 1

	ctx := auth.ContextWithUserID(context.Background(), testUserID)

	metadataCtx := app.NewMDContext(db.InMemory, false, false)
	tmdbAgent := agentsfakes.FakeMetadataRetrievalAgent{}
	metadataCtx.MetadataRetrievalAgent = &tmdbAgent
	r := NewResolver(metadataCtx)

	movieFile := db.MovieFile{
		MediaItem: db.MediaItem{
			FilePath: "/videos/North of the Sun.mkv",
		},
	}
	db.CreateMovieFile(&movieFile)

	tmdbAgent.UpdateMovieMetadataStub = func(movie *db.Movie) error {
		movie.Title = "North of the Sun"
		return nil
	}

	r.UpdateMovieFileMetadata(ctx,
		&struct{ Input UpdateMovieFileMetadataInput }{
			Input: UpdateMovieFileMetadataInput{
				MovieFileUUID: movieFile.UUID,
				TmdbID:        testTmdbID,
			},
		})

	// Check that the Movie model was created
	movies := db.FindAllMovies(&db.QueryDetails{Limit: 10})
	assert.Len(t, movies, 1)
	assert.Equal(t, testTmdbID, movies[0].TmdbID)
	assert.Equal(t, "North of the Sun", movies[0].Title)
}

func TestUpdateMovieFileUnknownTmdbID(t *testing.T) {
	const testTmdbID = 1234
	const testUserID = 1

	ctx := auth.ContextWithUserID(context.Background(), testUserID)

	metadataCtx := app.NewMDContext(db.InMemory, false, false)
	tmdbAgent := agentsfakes.FakeMetadataRetrievalAgent{}
	metadataCtx.MetadataRetrievalAgent = &tmdbAgent
	r := NewResolver(metadataCtx)

	movieFile := db.MovieFile{
		MediaItem: db.MediaItem{
			FilePath: "/videos/North of the Sun.mkv",
		},
	}
	db.CreateMovieFile(&movieFile)

	tmdbAgent.UpdateMovieMetadataStub = func(movie *db.Movie) error {
		// Don't modify for this test, the movie was not found in Tmdb.
		return errors.New("Not found")
	}

	responseResolver := r.UpdateMovieFileMetadata(ctx,
		&struct{ Input UpdateMovieFileMetadataInput }{
			Input: UpdateMovieFileMetadataInput{
				MovieFileUUID: movieFile.UUID,
				TmdbID:        testTmdbID,
			},
		})

	// Check that no movie was created
	// TODO(Leon Handreke): This currently filters by tmdb_id=0 and might therefore
	// conceal a Movie model being created
	//movies := db.FindAllMovies(&db.QueryDetails{Limit: 1})
	movieCount := -1
	metadataCtx.Db.Model(&db.Movie{}).Count(&movieCount)
	assert.Equal(t, 0, movieCount)

	assert.True(t, responseResolver.Error().HasError())
}
