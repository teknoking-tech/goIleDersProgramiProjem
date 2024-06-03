package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Öğrenci ders programı için model struct
type Schedule struct {
	ID        uint      `gorm:"primaryKey"`
	StudentID uint      `gorm:"not null"`
	Day       time.Time `gorm:"not null"`
	StartTime time.Time `gorm:"not null"`
	EndTime   time.Time `gorm:"not null"`
	State     string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Öğrenci için model struct
type Student struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"not null"`
	Email     string `gorm:"unique;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Echo örneğini başlatmak için fonksiyon
func initializeEcho(db *gorm.DB) *echo.Echo {
	e := echo.New()

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Öğrenci Ders Programı'na Hoş Geldiniz")
	})

	// Öğrenci CRUD endpointleri
	e.POST("/students", func(c echo.Context) error {
		var student Student
		if err := c.Bind(&student); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz istek verisi"})
		}
		if err := db.Create(&student).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Öğrenci kaydedilemedi"})
		}
		return c.JSON(http.StatusOK, student)
	})

	e.GET("/students/:id", func(c echo.Context) error {
		var student Student
		if err := db.First(&student, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Öğrenci bulunamadı"})
		}
		return c.JSON(http.StatusOK, student)
	})

	e.PUT("/students/:id", func(c echo.Context) error {
		var student Student
		if err := db.First(&student, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Öğrenci bulunamadı"})
		}
		if err := c.Bind(&student); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz istek verisi"})
		}
		if err := db.Save(&student).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Öğrenci güncellenemedi"})
		}
		return c.JSON(http.StatusOK, student)
	})

	e.DELETE("/students/:id", func(c echo.Context) error {
		if err := db.Delete(&Student{}, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Öğrenci silinemedi"})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Öğrenci başarıyla silindi"})
	})

	// Ders programı CRUD endpointleri
	e.POST("/schedule", func(c echo.Context) error {
		var scheduleData struct {
			StudentID uint   `json:"student_id"`
			Day       string `json:"day"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			State     string `json:"state"`
		}
		if err := c.Bind(&scheduleData); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz istek verisi"})
		}

		day, err := time.Parse("2006-01-02", scheduleData.Day)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz tarih formatı"})
		}
		startTime, err := time.Parse("15:04:05", scheduleData.StartTime)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz başlangıç zamanı formatı"})
		}
		endTime, err := time.Parse("15:04:05", scheduleData.EndTime)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz bitiş zamanı formatı"})
		}

		// Çakışan plan olup olmadığını kontrol et
		var conflict Schedule
		if err := db.Where("student_id = ? AND day = ? AND ((start_time BETWEEN ? AND ?) OR (end_time BETWEEN ? AND ?))", scheduleData.StudentID, day, startTime, endTime, startTime, endTime).First(&conflict).Error; err == nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "Belirtilen zaman aralığında zaten bir plan var"})
		}

		schedule := &Schedule{
			StudentID: scheduleData.StudentID,
			Day:       day,
			StartTime: startTime,
			EndTime:   endTime,
			State:     scheduleData.State,
		}
		if err := db.Create(schedule).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ders programı oluşturulamadı"})
		}
		return c.JSON(http.StatusOK, schedule)
	})

	e.GET("/schedule/:day", func(c echo.Context) error {
		dayString := c.Param("day")
		day, err := time.Parse("2006-01-02", dayString)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz tarih formatı"})
		}

		schedules, err := getSchedulesForDay(db, day)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ders programları alınamadı"})
		}
		return c.JSON(http.StatusOK, schedules)
	})

	e.PUT("/schedule/:id", func(c echo.Context) error {
		var schedule Schedule
		if err := db.First(&schedule, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Ders programı bulunamadı"})
		}
		if err := c.Bind(&schedule); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Geçersiz istek verisi"})
		}
		if err := db.Save(&schedule).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ders programı güncellenemedi"})
		}
		return c.JSON(http.StatusOK, schedule)
	})

	e.DELETE("/schedule/:id", func(c echo.Context) error {
		if err := db.Delete(&Schedule{}, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Ders programı silinemedi"})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Ders programı başarıyla silindi"})
	})

	return e
}

// Veritabanı bağlantısını başlatmak için fonksiyon
func initializeDatabase() (*gorm.DB, error) {
	dsn := "root:Fy913198.@tcp(localhost:3306)/ders_programi?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Belirli bir güne göre ders programlarını getirmek için fonksiyon
func getSchedulesForDay(db *gorm.DB, day time.Time) ([]Schedule, error) {
	var schedules []Schedule
	result := db.Where("day = ?", day).Find(&schedules)
	if result.Error != nil {
		return nil, result.Error
	}
	return schedules, nil
}

// HTTP POST isteği yapmak için örnek bir fonksiyon
func postSchedule() {
	url := "http://localhost:8080/schedule" // Endpoint URL'si

	// Gönderilecek veri
	data := map[string]interface{}{
		"student_id": 123,
		"day":        "2024-06-01",
		"start_time": "10:00:00",
		"end_time":   "12:00:00",
		"state":      "planlanıyor",
	}

	// Veriyi JSON formatına çevir
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Veri JSON formatına çevrilemedi:", err)
		return
	}

	// JSON verisini HTTP POST isteği ile gönder
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("HTTP isteği yapılamadı:", err)
		return
	}
	defer resp.Body.Close()

	// Yanıtı ekrana yazdır
	fmt.Println("HTTP Yanıt Kodu:", resp.Status)
}

// Ana fonksiyon
func main() {
	// Veritabanı bağlantısını başlat
	db, err := initializeDatabase()
	if err != nil {
		fmt.Println("Veritabanına bağlanılamadı:", err)
		return
	}
	// Modelleri migrate et
	if err := db.AutoMigrate(&Schedule{}, &Student{}); err != nil {
		fmt.Println("Tablo oluşturulamadı:", err)
		return
	}

	// Echo örneğini başlat
	e := initializeEcho(db)

	// Sunucuyu başlat
	go func() {
		if err := e.Start(":8080"); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server")
		}
	}()

	// HTTP POST isteğini yap
	postSchedule()

	// Sunucuyu kapatmak için sistem sinyallerini dinle
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Sunucuyu durdur
	if err := e.Shutdown(context.Background()); err != nil {
		e.Logger.Fatal(err.Error())
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
}
