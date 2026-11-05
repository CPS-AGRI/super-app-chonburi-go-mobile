package http

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"super-app-chonburi-go-mobile/config"
	"super-app-chonburi-go-mobile/internal/domain"
	"super-app-chonburi-go-mobile/pkg/jwtutil"
)

// chonburiLocations is the static master list of 11 districts and their subdistricts.
// Served as-is — no DB query needed, embedded from the same source as the Mobile UI.
var chonburiLocations = []domain.ChonburiLocationItem{
	{District: "เมืองชลบุรี", Subdistricts: []string{"บางปลาสร้อย", "หนองไม้แดง", "มะขามหย่ง", "บางทราย", "บ้านโขด", "แสนสุข", "เหมือง", "นาป่า", "อ่างศิลา", "เสม็ด", "บ้านสวน", "ดอนหัวฬ่อ", "หนองรี", "หนองข้างคอก", "สำนักบก"}},
	{District: "ศรีราชา", Subdistricts: []string{"ศรีราชา", "สุรศักดิ์", "ทุ่งสุขลา", "บ่อวิน", "หนองขาม", "เขาคันทรง", "บางพระ"}},
	{District: "บางละมุง", Subdistricts: []string{"หนองปรือ", "นาเกลือ", "ห้วยใหญ่", "หนองปลาไหล", "โป่ง", "เขาไม้แก้ว", "ตะเคียนเตี้ย"}},
	{District: "สัตหีบ", Subdistricts: []string{"สัตหีบ", "นาจอมเทียน", "บางเสร่", "พลูตาหลวง", "แสมสาร"}},
	{District: "พานทอง", Subdistricts: []string{"พานทอง", "หนองตำลึง", "มาบโป่ง", "หนองกะขะ", "บ้านเก่า", "เกาะลอย"}},
	{District: "พนัสนิคม", Subdistricts: []string{"พนัสนิคม", "หน้าพระธาตุ", "วัดหลวง", "บ้านเซิด", "หนองปรือ"}},
	{District: "บ้านบึง", Subdistricts: []string{"บ้านบึง", "คลองกิ่ว", "หนองไผ่แก้ว", "หนองชาก", "หนองบอนแดง"}},
	{District: "เกาะสีชัง", Subdistricts: []string{"ท่าเทววงษ์"}},
	{District: "บ่อทอง", Subdistricts: []string{"บ่อทอง", "วัดสุวรรณ", "บ่อกวางทอง", "เกษตรสุวรรณ"}},
	{District: "หนองใหญ่", Subdistricts: []string{"หนองใหญ่", "คลองพลู", "หนองเสือช้าง", "ห้างสูง"}},
	{District: "เกาะจันทร์", Subdistricts: []string{"เกาะจันทร์", "ท่าบุญมี", "สระสี่เหลี่ยม"}},
}

// WeatherSubscriptionHandler handles mobile API requests for weather area subscriptions.
type WeatherSubscriptionHandler struct {
	useCase domain.WeatherSubscriptionUseCase
}

// RegisterWeatherSubscriptionRoutes mounts all subscription API routes.
func RegisterWeatherSubscriptionRoutes(app *fiber.App, cfg *config.Config, uc domain.WeatherSubscriptionUseCase) {
	h := &WeatherSubscriptionHandler{useCase: uc}

	// Register on both /api/v1/mobile/weather and /api/v1/weather for client compatibility
	registerGroup := func(g fiber.Router) {
		g.Get("/subscriptions", h.GetSubscriptions)
		g.Post("/subscriptions", h.AddSubscription)
		g.Delete("/subscriptions/:id", h.RemoveSubscription)
		g.Get("/alerts", h.GetAlerts)
	}

	registerGroup(app.Group("/api/v1/mobile/weather", jwtutil.RequireAuth(cfg)))
	registerGroup(app.Group("/api/v1/weather", jwtutil.RequireAuth(cfg)))

	// Static data — no auth required, cached at client
	app.Get("/api/v1/mobile/weather/locations", h.GetLocations)
	app.Get("/api/v1/weather/locations", h.GetLocations)
}

// GetSubscriptions godoc
// @Summary     Get user's weather area subscriptions
// @Tags        Weather Subscriptions
// @Security    BearerAuth
// @Produce     json
// @Success     200 {array}  domain.WeatherSubscriptionResponse
// @Router      /api/v1/mobile/weather/subscriptions [get]
func (h *WeatherSubscriptionHandler) GetSubscriptions(c fiber.Ctx) error {
	userID, err := extractUserUUID(c)
	if err != nil {
		return fiber.ErrUnauthorized
	}

	subs, err := h.useCase.GetSubscriptions(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": subs})
}

// AddSubscription godoc
// @Summary     Add a weather area subscription
// @Tags        Weather Subscriptions
// @Security    BearerAuth
// @Accept      json
// @Produce     json
// @Param       body body domain.WeatherSubscriptionRequest true "Subscription"
// @Success     201 {object} fiber.Map
// @Router      /api/v1/mobile/weather/subscriptions [post]
func (h *WeatherSubscriptionHandler) AddSubscription(c fiber.Ctx) error {
	userID, err := extractUserUUID(c)
	if err != nil {
		return fiber.ErrUnauthorized
	}

	var req domain.WeatherSubscriptionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.useCase.AddSubscription(userID, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "เพิ่มพื้นที่รับแจ้งเตือนสำเร็จ"})
}

// RemoveSubscription godoc
// @Summary     Remove a secondary weather area subscription
// @Tags        Weather Subscriptions
// @Security    BearerAuth
// @Param       id path string true "Subscription ID (UUID)"
// @Produce     json
// @Success     200 {object} fiber.Map
// @Router      /api/v1/mobile/weather/subscriptions/{id} [delete]
func (h *WeatherSubscriptionHandler) RemoveSubscription(c fiber.Ctx) error {
	userID, err := extractUserUUID(c)
	if err != nil {
		return fiber.ErrUnauthorized
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid subscription id"})
	}

	if err := h.useCase.RemoveSubscription(id, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "ลบพื้นที่รับแจ้งเตือนสำเร็จ"})
}

// GetLocations godoc
// @Summary     Get all Chonburi districts and subdistricts (for picker UI)
// @Tags        Weather Subscriptions
// @Produce     json
// @Success     200 {array} domain.ChonburiLocationItem
// @Router      /api/v1/mobile/weather/locations [get]
func (h *WeatherSubscriptionHandler) GetLocations(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": chonburiLocations})
}

// GetAlerts godoc
// @Summary     Get recent weather alerts for user's subscribed areas
// @Tags        Weather Subscriptions
// @Security    BearerAuth
// @Produce     json
// @Success     200 {object} fiber.Map
// @Router      /api/v1/mobile/weather/alerts [get]
func (h *WeatherSubscriptionHandler) GetAlerts(c fiber.Ctx) error {
	userID, err := extractUserUUID(c)
	if err != nil {
		return fiber.ErrUnauthorized
	}

	limit, _ := strconv.Atoi(c.Query("limit", "30"))
	if limit <= 0 {
		limit = 30
	}
	alerts, err := h.useCase.GetAlertHistory(userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": alerts})
}

// extractUserUUID is a shared helper to extract and parse the authenticated user's UUID.
func extractUserUUID(c fiber.Ctx) (uuid.UUID, error) {
	userIDStr, err := jwtutil.ExtractUserID(c)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(userIDStr)
}
