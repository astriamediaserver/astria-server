package metadata

import (
	"fmt"
	"github.com/Jeffail/tunny"
	"gitlab.com/bytesized/bytesized-streaming/metadata/db"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var supportedExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".mov": true,
	".avi": true,
}

const (
	MediaTypeMovie = iota
	MediaTypeSeries
	MediaTypeMusic
	MediaTypeOtherMovie
)

type LibraryManager struct {
	ctx  *db.MetadataContext
	pool *tunny.Pool
}

type EpisodePayload struct {
	series  db.TvSeries
	season  db.TvSeason
	episode db.TvEpisode
}

func NewLibraryManager(ctx *db.MetadataContext) *LibraryManager {
	manager := LibraryManager{ctx: ctx}
	manager.pool = tunny.NewFunc(4, func(payload interface{}) interface{} {
		fmt.Println("Starting worker")
		ep, ok := payload.(EpisodePayload)
		if ok {
			fmt.Printf("Worker in '%s', S%dE%s\n", ep.series.Name, ep.season.SeasonNumber, ep.episode.EpisodeNum)
			err := manager.UpdateEpisodeMD(ep.series, ep.season, ep.episode)
			if err != nil {
				fmt.Println("GOT AN ERROR UPDATING EPISODE")
			}
		}
		fmt.Println("Ending worker")
		return nil
	})
	manager.pool.SetSize(10)

	return &manager
}

func (self *LibraryManager) UpdateMD(library *db.Library) {
	switch kind := library.Kind; kind {
	case MediaTypeMovie:
		fmt.Println("Updating meta-data for movies")
		self.UpdateMovieMD(library)
	case MediaTypeSeries:
		fmt.Println("Updating meta-data for TV")
		self.UpdateTvMD(library)
	}
}

func (self *LibraryManager) UpdateEpisodeMD(tv db.TvSeries, season db.TvSeason, episode db.TvEpisode) error {
	fmt.Printf("Grabbing metadata for episode %s for series '%s'\n", episode.EpisodeNum, tv.Name)
	episodeInt, err := strconv.ParseInt(episode.EpisodeNum, 10, 32)
	if err != nil {
		fmt.Println("Could not parse season:", err)
	}
	fullEpisode, err := self.ctx.Tmdb.GetTvEpisodeInfo(tv.TmdbID, season.SeasonNumber, int(episodeInt), nil)
	if err == nil {
		if fullEpisode != nil {
			episode.SetUUID()
			episode.AirDate = fullEpisode.AirDate
			episode.Name = fullEpisode.Name
			episode.TmdbID = fullEpisode.ID
			episode.Overview = fullEpisode.Overview
			episode.StillPath = fullEpisode.StillPath
			obj := self.ctx.Db.Save(&episode)
			return obj.Error
		}
		return nil
	} else {
		fmt.Println("Could not grab episode information:", err)
		return err
	}
}

func (self *LibraryManager) UpdateEpisodesMD() error {
	episodes := []db.TvEpisode{}
	self.ctx.Db.Where("tmdb_id = ?", 0).Find(&episodes)
	for _, episode := range episodes {
		var season db.TvSeason
		var tv db.TvSeries
		self.ctx.Db.Where("id = ?", episode.TvSeasonID).Find(&season)
		self.ctx.Db.Where("id = ?", season.TvSeriesID).Find(&tv)
		go func() {
			self.pool.Process(EpisodePayload{season: season, series: tv, episode: episode})
		}()
	}
	return nil
}

func (self *LibraryManager) UpdateSeasonMD() error {
	seasons := []db.TvSeason{}
	self.ctx.Db.Where("tmdb_id = ?", 0).Find(&seasons)
	for _, season := range seasons {
		var tv db.TvSeries
		self.ctx.Db.Where("id = ?", season.TvSeriesID).Find(&tv)

		fmt.Printf("Grabbing meta-data for season %d of series '%s'\n", season.SeasonNumber, tv.Name)
		fullSeason, err := self.ctx.Tmdb.GetTvSeasonInfo(tv.TmdbID, season.SeasonNumber, nil)
		if err == nil {
			season.SetUUID()
			season.AirDate = fullSeason.AirDate
			season.Overview = fullSeason.Overview
			season.Name = fullSeason.Name
			season.TmdbID = fullSeason.ID
			season.PosterPath = fullSeason.PosterPath
			self.ctx.Db.Save(&season)
		} else {
			fmt.Println("Could not grab seasonal information")
		}
	}
	return nil
}

func (self *LibraryManager) UpdateTvMD(library *db.Library) error {
	series := []db.TvSeries{}
	self.ctx.Db.Where("tmdb_id = ?", 0).Find(&series)
	for _, serie := range series {
		fmt.Println("Looking up meta-data for series:", serie.Name)
		var options = make(map[string]string)
		if serie.FirstAirYear != 0 {
			options["first_air_date_year"] = strconv.FormatUint(serie.FirstAirYear, 10)
		}
		searchRes, err := self.ctx.Tmdb.SearchTv(serie.Name, options)

		if err != nil {
			return err
		}

		if len(searchRes.Results) > 0 {
			fmt.Println("Found Series that matches, using first result and doing deepscan.")
			tv := searchRes.Results[0] // Take the first result for now
			fullTv, err := self.ctx.Tmdb.GetTvInfo(tv.ID, nil)
			if err == nil {
				serie.Overview = fullTv.Overview
				serie.Status = fullTv.Status
				serie.Type = fullTv.Type
			} else {
				fmt.Println("Could not get full results, only adding search results. Error:", err)
			}
			serie.SetUUID()
			serie.TmdbID = tv.ID
			serie.FirstAirDate = tv.FirstAirDate
			serie.OriginalName = tv.OriginalName
			serie.BackdropPath = tv.BackdropPath
			serie.PosterPath = tv.PosterPath
			self.ctx.Db.Save(&serie)
		}
	}

	self.UpdateSeasonMD()
	self.UpdateEpisodesMD()
	//episodes := []db.TvEpisode{}
	//self.ctx.Db.Where("tmdb_id = ? AND library_id = ?", 0, library.ID).Find(&episodes)
	//for _, episode := range episodes {
	//}

	return nil
}

func (self *LibraryManager) UpdateMovieMD(library *db.Library) error {
	movies := []db.MovieItem{}
	self.ctx.Db.Where("tmdb_id = ? AND library_id = ?", 0, library.ID).Find(&movies)
	for _, movie := range movies {
		fmt.Printf("Attempting to fetch metadata for '%s'\n", movie.Title)
		var options = make(map[string]string)
		if movie.Year > 0 {
			options["year"] = movie.YearAsString()
		}
		searchRes, err := self.ctx.Tmdb.SearchMovie(movie.Title, options)

		if err != nil {
			return err
		}

		if len(searchRes.Results) > 0 {
			fmt.Println("Found movie that matches, using first result and doing deepscan.")
			mov := searchRes.Results[0] // Take the first result for now
			fullMov, err := self.ctx.Tmdb.GetMovieInfo(mov.ID, nil)
			if err == nil {
				movie.Overview = fullMov.Overview
				movie.ImdbID = fullMov.ImdbID
			} else {
				fmt.Println("Could not get full results, only adding search results. Error:", err)
			}
			movie.SetUUID()
			movie.TmdbID = mov.ID
			movie.ReleaseDate = mov.ReleaseDate
			movie.OriginalTitle = mov.OriginalTitle
			movie.BackdropPath = mov.BackdropPath
			movie.PosterPath = mov.PosterPath
			self.ctx.Db.Save(&movie)
		}

	}
	return nil
}

func (self *LibraryManager) Probe(library *db.Library) {
	switch kind := library.Kind; kind {
	case MediaTypeMovie:
		fmt.Println("Probing for movies")
		self.ProbeMovies(library)
	case MediaTypeSeries:
		fmt.Println("Probing for series")
		self.ProbeSeries(library)
	}
}

func (self *LibraryManager) ProbeSeries(library *db.Library) {
	err := filepath.Walk(library.FilePath, func(walkPath string, info os.FileInfo, err error) error {
		if supportedExtensions[filepath.Ext(walkPath)] {
			count := 0
			self.ctx.Db.Where("file_path= ?", walkPath).Find(&db.TvEpisode{}).Count(&count)
			if count == 0 {
				fileInfo, err := os.Stat(walkPath)

				if err != nil {
					// This catches broken symlinks
					if _, ok := err.(*os.PathError); ok {
						fmt.Println("Got an error while statting file:", err)
						return nil
					}
					return err
				}
				var year string
				fileName := fileInfo.Name()
				// First figure out if there is a year in there, and if so parse it out.
				yearRegex := regexp.MustCompile("([\\[\\(]?((?:19[0-9]|20[01])[0-9])[\\]\\)]?)")
				ress := yearRegex.FindStringSubmatch(fileInfo.Name())
				if len(ress) > 1 {
					year = ress[2]
					fmt.Println(year)
					fileName = strings.Replace(fileName, ress[1], "", -1)
				}
				seriesRe := regexp.MustCompile("^(.*)S(\\d{2})E(\\d{2})")
				res := seriesRe.FindStringSubmatch(fileName)

				if len(res) > 2 {
					yearInt, err := strconv.ParseUint(res[2], 10, 32)
					if err != nil {
						fmt.Println("Could not parse year:", err)
					}

					title := Sanitize(res[1])
					season := res[2]
					episode := res[3]
					fmt.Printf("Found '%s' season %s episode %s\n", title, season, episode)
					mi := db.MediaItem{
						FileName:  fileInfo.Name(),
						FilePath:  walkPath,
						Size:      fileInfo.Size(),
						Title:     title,
						LibraryID: library.ID,
						Year:      yearInt,
					}
					var tv db.TvSeries
					var tvs db.TvSeason

					seasonInt, err := strconv.ParseInt(season, 10, 32)
					if err != nil {
						fmt.Println("Could not parse season:", err)
					}

					self.ctx.Db.FirstOrCreate(&tv, db.TvSeries{Name: title, FirstAirYear: yearInt})
					self.ctx.Db.FirstOrCreate(&tvs, db.TvSeason{TvSeriesID: tv.ID, Name: title, SeasonNumber: int(seasonInt)})

					ep := db.TvEpisode{MediaItem: mi, SeasonNum: season, EpisodeNum: episode, TvSeasonID: tvs.ID}
					self.ctx.Db.Create(&ep)
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}

func (self *LibraryManager) ProbeMovies(library *db.Library) {
	err := filepath.Walk(library.FilePath, func(walkPath string, info os.FileInfo, err error) error {
		var title string
		var year uint64

		if err != nil {
			return err
		}
		if supportedExtensions[filepath.Ext(walkPath)] {
			count := 0
			self.ctx.Db.Where("file_path= ?", walkPath).Find(&db.MovieItem{}).Count(&count)
			if count == 0 {
				fileInfo, err := os.Stat(walkPath)

				if err != nil {
					// This catches broken symlinks
					if _, ok := err.(*os.PathError); ok {
						fmt.Println("Got an error while statting file:", err)
						return nil
					}
					return err
				}

				movieRe := regexp.MustCompile("(.*)\\((\\d{4})\\)")
				res := movieRe.FindStringSubmatch(fileInfo.Name())

				if len(res) > 1 {
					title = Sanitize(res[1])
				}
				if len(res) > 2 {
					year, err = strconv.ParseUint(res[2], 10, 32)
					if err != nil {
						fmt.Println("Could not parse year:", err)
					}
				}

				if title == "" {
					basename := fileInfo.Name()
					name := strings.TrimSuffix(basename, filepath.Ext(basename))
					fmt.Println("Could not parse title for:")
					fmt.Println("Trying heavy sanitizing")
					var yearStr string
					title, yearStr = HeavySanitize(name)
					year, err = strconv.ParseUint(yearStr, 10, 32)
					if err != nil {
						fmt.Println("Could not parse year:", err)
					}
					fmt.Println("attempted to find some stuff", title, year)
				}

				mi := db.MediaItem{
					FileName:  fileInfo.Name(),
					FilePath:  walkPath,
					Size:      fileInfo.Size(),
					Title:     title,
					Year:      year,
					LibraryID: library.ID,
				}
				movie := db.MovieItem{MediaItem: mi}
				fmt.Println(movie.String())
				self.ctx.Db.Create(&movie)
			} else {
				fmt.Printf("Path '%s' already exists in library.\n", walkPath)
			}
		}

		return nil
	})

	if err != nil {
		fmt.Println(err)
		return

	}
}

func (self *LibraryManager) ActivateAll() {
	for _, lib := range db.AllLibraries() {
		fmt.Println("Scanning library:", lib.Name, lib.FilePath)
		self.Probe(&lib)

		fmt.Println("Updating metadata for library:", lib.Name)
		self.UpdateMD(&lib)
	}
}