package v1

import (
	"errors"
	"github.com/gin-gonic/gin"
	"go-sso/db/inter"
	"go-sso/db/model"
	"go-sso/pkg/email_tool"
	"go-sso/pkg/log"
	"go-sso/pkg/storage"
	"go-sso/service/api/api_error"
	"go-sso/service/api/viewset"
	"go-sso/service/middlewares"
	"go-sso/util"
	"net/http"
)

type AuthViewset struct {
	itemInter inter.IUser
	viewset.ViewSet
}

func (a *AuthViewset) ErrorHandler(f func(c *gin.Context) error) func(c *gin.Context) {
	return func(c *gin.Context) {
		a.ViewSet.ErrorHandler(f, c)
	}
}

// @Summary user login
// @Description 1.账号密码登录 2.手机号，邮箱登录
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/login/ [post]
func (a *AuthViewset) Login(c *gin.Context) (err error) {
	var up model.UserParams
	err = c.ShouldBind(&up)
	if err != nil {
		log.Error(err.Error())
		a.FailResponse(c, api_error.ErrInvalid)
		return api_error.ErrInvalid
	}
	u, r := a.itemInter.CheckUser(up.Account, up.Password)
	if !r {
		return api_error.ErrAuth
	}
	// 登录方式 token
	driver := middlewares.GenerateAuthDriver(middlewares.TokenAuth)
	res := driver.Login(c, u)

	query := c.Query("redirect_url")
	if query != "" {
		m, ok := res.(map[string]interface{})
		if !ok {
			return api_error.ErrInternal
		}
		url, err := util.BuildUrlQuery(query, m)
		if err != nil {
			log.Errorf("%+v", err)
			return api_error.ErrInternal
		}
		c.Redirect(http.StatusMovedPermanently, url)
		return nil
	}

	return a.SuccessResponse(c, res)
}

// @Summary telephone login
// @Description telephone login steps 1.IsTelephoneExist 2. SendSmsCode 3. TelephoneLogin
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/telephone/login/ [post]
func (a *AuthViewset) TelephoneLogin(c *gin.Context) (err error) {
	var tl model.TelephoneLoginParams
	if err = c.ShouldBind(&tl); err != nil {
		return err
	}
	err = a.VerifySmsCode(tl.Telephone, tl.Code)
	if err != nil {
		return
	}
	// 登录方式 token
	u, err := a.itemInter.GetUserByAccount(tl.Telephone)
	if err != nil {
		return
	}
	driver := middlewares.GenerateAuthDriver(middlewares.TokenAuth)
	res := driver.Login(c, u)
	return a.SuccessResponse(c, res)
}

// @Summary user register
// @Description register by username, telephone and password
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/register/ [post]
func (a *AuthViewset) Register(c *gin.Context) (err error) {
	var rp model.RegisterParams
	var user model.User
	err = c.ShouldBind(&rp)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	err = rp.Validate()
	if err != nil {
		return err
	}
	errs := a.CheckRegisterParams(&rp)
	if len(errs) > 0 {
		a.FailResponse(c, api_error.ErrInvalid, errs)
		return
	}
	newPassword, err := util.GeneratePassword(rp.Password)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	user.Username = rp.Username
	user.Telephone = rp.Telephone
	user.Email = rp.Email
	user.Password = newPassword
	if _, err = a.itemInter.Create(&user); err != nil {
		log.Error(err.Error())
		return err
	} else {
		return a.SuccessResponse(c, "注册成功")
	}
}

// 检测注册用户参数
func (a *AuthViewset) CheckRegisterParams(rp *model.RegisterParams) map[string]string {
	errs := make(map[string]string)
	// 检查参数是否合法
	if !a.itemInter.IsValid(rp.Username, "username") {
		errs["username"] = "用户名至少3位以上字母开头"
	}
	if !a.itemInter.IsValid(rp.Telephone, "telephone") {
		errs["telephone"] = "手机号格式错误"
	}
	if rp.Email != "" && !a.itemInter.IsValid(rp.Email, "email") {
		errs["email"] = "email格式错误"
	}
	if len(errs) > 0 {
		return errs
	}
	// 检查是否重复注册
	if a.itemInter.Exists(rp.Username, "username") {
		errs["username"] = "用户已经存在"
	}
	if a.itemInter.Exists(rp.Telephone, "telephone") {
		errs["telephone"] = "手机号已经存在"
	}
	if rp.Email != "" && a.itemInter.Exists(rp.Email, "email") {
		errs["email"] = "email已经存在"
	}
	return errs
}

// @Summary 账号注册手机号校验是否合法
// @Description send telephone verify code
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/check-telephone/ [post]
func (a *AuthViewset) CheckTelephoneValid(c *gin.Context) (err error) {
	errs := make(map[string]string)
	// 检查参数是否合法
	telephone := c.Query("telephone")
	if !a.itemInter.IsValid(telephone, "telephone") {
		errs["telephone"] = "手机号格式错误"
	}
	if a.itemInter.Exists(telephone, "telephone") {
		errs["telephone"] = "手机号已经存在"
	}
	if len(errs) != 0 {
		a.FailResponse(c, api_error.ErrInvalid, errs)
		return
	}
	return a.SuccessBlankResponse(c)
}

// @Summary check telephone
// @Description check telephone whether exist
// @Accept  json
// @Produce  json
// @Param  user body  true "telephone"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/check-telephone-exist/ [post]
func (a *AuthViewset) CheckTelephoneExist(c *gin.Context) (err error) {
	errs := make(map[string]string)
	// 检查参数是否合法
	telephone := c.Query("telephone")
	if !a.itemInter.IsValid(telephone, "telephone") {
		errs["telephone"] = "手机号格式错误"
	}
	if !a.itemInter.Exists(telephone, "telephone") {
		errs["telephone"] = "手机号不存在"
	}
	if len(errs) != 0 {
		a.FailResponse(c, api_error.ErrInvalid, errs)
		return
	}
	return a.SuccessBlankResponse(c)
}

// @Summary 发送手机验证码
// @Description send telephone verify code
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/register/ [post]
func (a *AuthViewset) SendSmsCode(c *gin.Context) (err error) {
	// TODO 发送短信
	code := "123456"
	telephone := c.Query("telephone")
	if ok := a.itemInter.IsValid(telephone, "telephone"); !ok {
		return api_error.ErrInvalid
	}
	cacheStore := storage.GetStore()
	cacheStore.SetCache(telephone, code)
	return a.SuccessBlankResponse(c)
}

// @Summary telephone check
// @Description 手机验证码确认
func (a *AuthViewset) VerifySmsCode(telephone, code string) (err error) {
	cacheStore := storage.GetStore()
	if rightCode, ok := cacheStore.GetCache(telephone); ok && rightCode.(string) == code {
		return
	}
	return nil
}

// @Summary 发送邮箱验证码
// @Description register by username and password
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/send-email-code/ [post]
func (a *AuthViewset) SendEmailCode(c *gin.Context) (err error) {
	email := c.Query("email")
	if ok := a.itemInter.IsValid(email, "email"); !ok {
		return api_error.ErrInvalid
	}
	cacheStore := storage.GetStore()
	code := util.RandomCode()
	err = email_tool.SendEmailCode(code, email)
	if err != nil {
		return err
	}
	cacheStore.SetCache(email, code)
	return a.SuccessResponse(c, gin.H{"url": ""})
}

func (a *AuthViewset) VerifyEmailCode(email, code string) (err error) {
	cacheStore := storage.GetStore()
	if rightCode, ok := cacheStore.GetCache(email); ok && rightCode.(string) == code {
		return
	}
	return nil
}

// @Summary 重置密码
// @Description reset password by telephone or email
// @Accept  json
// @Produce  json
// @Param  user body model.UserParams true "username && password"
// @Success 200 {object} viewset.Response
// @Router /api/public/v1/auth/reset-password/ [post]
func (a *AuthViewset) ResetPassword(c *gin.Context) (err error) {
	var rp model.ResetPasswordParams
	err = c.ShouldBind(&rp)
	if err != nil {
		return
	}
	user, err := a.itemInter.GetUserByAccount(rp.Account)
	if err != nil {
		return
	}
	switch rp.VerifyType {
	case "email":
		err = a.VerifyEmailCode(user.Email, rp.Code)
	case "telephone":
		err = a.VerifySmsCode(user.Telephone, rp.Code)
	default:
		err = errors.New("验证类型错误")
	}
	if err != nil {
		return
	}
	err = a.itemInter.ChangePassword(user, rp.NewPassword)
	if err != nil {
		return
	}
	return a.SuccessBlankResponse(c)
}

// @Summary 修改密码
// @Description change password by username and password
// @Accept  json
// @Produce  json
// @Param  user body model.ChangePasswordParams true "raw_password && new_password"
// @Success 200 {object} viewset.Response
// @Router /api/v1/auth/change-password/ [post]
func (a *AuthViewset) ChangePassword(c *gin.Context) (err error) {
	var cp model.ChangePasswordParams
	err = c.ShouldBind(&cp)
	if err != nil {
		log.Error(err)
		return err
	}
	err = cp.Validate()
	if err != nil {
		log.Error(err)
		return err
	}
	username := middlewares.GetCurrentUser(c).Username
	if u, ok := a.itemInter.CheckUser(username, cp.RawPassword); ok {
		err = a.itemInter.ChangePassword(u, cp.NewPassword)
		if err != nil {
			log.Error(err)
			return
		}
		return a.SuccessBlankResponse(c)
	}
	return errors.New("原密码错误")
}
