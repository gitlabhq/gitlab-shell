package twofactorverify

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/twofactorverify"
)

type Command struct {
	Config     *config.Config
	Client     *twofactorverify.Client
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type Result struct {
	Error   error
	Status  string
	Success bool
}

var (
	mu              sync.RWMutex
	// TODO: make timeout configurable
	ctxMaxTime = time.Second + 30
)

func (c *Command) Execute2(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)

	// config.GetHTTPClient isn't thread-safe so save Client in struct for concurrency
	// workaround until #518 is fixed
	var err error
	c.Client, err = twofactorverify.NewClient(c.Config)
	fmt.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	fmt.Println("client = ", c.Client)
	fmt.Println("err = ", err)
	fmt.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	if err != nil {
		ctxlog.WithError(err).Error("twofactorverify: execute: OTP verification failed")
		return err
	}

	// Create timeout context
	// TODO: make timeout configurable
	const ctxTimeout = 30
	timeoutCtx, cancelTimeout := context.WithTimeout(ctx, ctxTimeout*time.Second)
	verifyCtx, cancelVerify := context.WithCancel(timeoutCtx)
	pushCtx, cancelPush := context.WithCancel(timeoutCtx)
	defer cancelTimeout()
	//fmt.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	//fmt.Println(verifyCtx, ", ", cancelVerify)
	//fmt.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	// Background push notification with timeout
	pushauth := make(chan Result)
	go func() {
		defer close(pushauth)
		status, success, err := c.pushAuth(pushCtx)

		select {
		case <-pushCtx.Done(): // push cancelled by manual OTP
			pushauth <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			pushauth <- Result{Error: err, Status: status, Success: success}
			cancelVerify()
		}
	}()

	// Also allow manual OTP entry while waiting for push, with same timeout as push
	verify := make(chan Result)
	go func() {
		defer close(verify)
		ctxlog.Info("twofactorverify: execute: waiting for user input")
		answer := ""
		answer = c.getOTP(verifyCtx)

		select {
		case <-verifyCtx.Done(): // manual OTP cancelled by push
			verify <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			cancelPush()
			ctxlog.Info("twofactorverify: execute: verifying entered OTP")
			status, success, err := c.verifyOTP(verifyCtx, answer)
			//fmt.Println("-------------")
			//fmt.Println("pushAuth.status = ", status)
			//fmt.Println("pushAuth.success = ", success)
			//fmt.Println("pushAuth.err = ", err)
			//fmt.Println("-------------")
			ctxlog.WithError(err).Info("twofactorverify: execute: OTP verified")
			verify <- Result{Error: err, Status: status, Success: success}
		}
	}()


	for {
		select {
		case res := <-verify: // manual OTP
			//fmt.Println("-------------")
			//fmt.Println("verify.res = ", res)
			//fmt.Println("-------------")
			if res.Status == "cancelled" {
				// verify cancelled; don't print anything
			} else if res.Status == "" {
				// channel closed; don't print anything
			} else {
				fmt.Fprint(c.ReadWriter.Out, res.Status)
				return nil
			}
		case res := <-pushauth: // push
			//fmt.Println("-------------")
			//fmt.Println("pushauth.res = ", res)
			//fmt.Println("-------------")
			if res.Status == "cancelled" {
				// push cancelled; don't print anything
			} else if res.Status == "" {
				// channel closed; don't print anything
			} else {
				fmt.Fprint(c.ReadWriter.Out, res.Status)
				return nil
			}
		case <-timeoutCtx.Done(): // push timed out
			fmt.Fprint(c.ReadWriter.Out, "\nOTP verification timed out\n")
			return nil
		}
	}

	return nil
}
func (c *Command) Execute(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)

	// config.GetHTTPClient isn't thread-safe so save Client in struct for concurrency
	// workaround until #518 is fixed
	var err error
	c.Client, err = twofactorverify.NewClient(c.Config)

	if err != nil {
		ctxlog.WithError(err).Error("twofactorverify: execute: OTP verification failed")
		return err
	}

	waitGroup := sync.WaitGroup{}

	myctx, cancelCtx := context.WithTimeout(ctx, ctxMaxTime)
	defer cancelCtx()
	//myctx, mycancel := context.WithCancel(timeoutCtx)



	// Also allow manual OTP entry while waiting for push, with same timeout as push
	otpChannel := make(chan Result)
	waitGroup.Add(1)
	//defer close(otpChannel)

	go func() {
		defer waitGroup.Done()
		ctxlog.Info("twofactorverify: execute: waiting for user input")
		otpAnswer := c.getOTP(myctx)

		select {
		case <-ctx.Done(): // manual OTP cancelled by push
			otpChannel <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			status, success, err := c.verifyOTP(myctx, otpAnswer)
			otpChannel <- Result{Error: err, Status: status, Success: success}
		}
		//cancelCtx()
	}()
	//// Background push notification with timeout
	pushChannel := make(chan Result)
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		//defer close(pushChannel)
		ctxlog.Info("twofactorverify: execute: waiting for push auth")
		//status, success, err := c.pushAuth(myctx)
		//ctxlog.WithError(err).Info("twofactorverify: execute: push auth verified")

		select {
		case <-myctx.Done(): // push cancelled by manual OTP
			// skip writing to channel
			pushChannel <- Result{Error: nil, Status: "cancelled", Success: false}
			ctxlog.Info("twofactorverify: execute: push auth cancelled")
		//default:
		//	pushChannel <- Result{Error: err, Status: status, Success: success}
		}
	}()
	//for {
		select {
			case res := <-otpChannel:
				//fmt.Println("Received from otpChannel => ", res)
				if len(res.Status) > 0 && res.Status != "cancelled" {
					fmt.Fprint(c.ReadWriter.Out, res.Status)
					return nil
				}
			case res := <-pushChannel:
			    if len(res.Status) > 0 && res.Status != "cancelled" {
					//fmt.Println("Received from pushChannel => ", res)
					fmt.Println("res.Status == ", res.Status, " -> ", len(res.Status))
			//		//fmt.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
			//		//fmt.Println(res)
			//		//fmt.Println("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
					fmt.Fprint(c.ReadWriter.Out, res.Status)
					return nil
				}
			//case <- myctx.Done():
			//	fmt.Fprint(c.ReadWriter.Out, "\nOTP verification timed out\n")
			//	return nil

		}

	waitGroup.Wait()
	//}

	return nil
}
//
//func (c Command) processCmd(ctx context.Context, cancelTimeout context.CancelFunc) (result Result) {
//	ctxlog := log.ContextLogger(ctx)
//
//	otpAuth := make(chan Result)
//	go func() {
//		defer close(otpAuth)
//		ctxlog.Info("twofactorverify: execute: waiting for user input")
//		otpAnswer := c.getOTP(ctx)
//
//		select {
//			case <-ctx.Done(): // manual OTP cancelled by push
//				fmt.Println("otpAuth.ctx.Done()")
//				otpAuth <- Result{Error: nil, Status: "cancelled", Success: false}
//				fmt.Println("----------------------------------------------------")
//				fmt.Println("otpAuth = ", otpAuth)
//				fmt.Println("----------------------------------------------------")
//			default:
//				fmt.Println("otpAuth.default")
//				cancelTimeout()
//				fmt.Println("Call c.verifyOTP(", ctx, ", ", otpAnswer, ")")
//				status, success, err := c.verifyOTP(ctx, otpAnswer)
//				fmt.Println("otpAnswer.status = ", status)
//				fmt.Println("otpAnswer.success = ", success)
//				fmt.Println("otpAnswer.err = ", err)
//				otpAuth <- Result{Error: err, Status: status, Success: success}
//				fmt.Println("----------------------------------------------------")
//				fmt.Println("otpAuth = ", otpAuth)
//				fmt.Println("----------------------------------------------------")
//		}
//	}()
//	for {
//		//fmt.Println("for loop")
//		select {
//			case res := <- otpAuth:
//				fmt.Println(res)
//				//fmt.Println("-------------")
//				//fmt.Println("otpAuth = ", ores)
//				//fmt.Println("-------------")
//				if len(res.Status) > 0 && res.Status != "cancelled"{
//					//fmt.Println("-------------")
//					//fmt.Println("otpAuth = ", res.Status)
//					//fmt.Println("-------------")
//					return res
//				}
//		}
//	}
//	return
//}
//
//


func (c *Command) getOTP(ctx context.Context) string {
	prompt := "OTP: "
	fmt.Fprint(c.ReadWriter.Out, prompt)

	var answer string
	otpLength := int64(64)
	reader := io.LimitReader(c.ReadWriter.In, otpLength)
	if _, err := fmt.Fscanln(reader, &answer); err != nil {
		log.ContextLogger(ctx).WithError(err).Debug("twofactorverify: getOTP: Failed to get user input")
	}

	return answer
}


func (c *Command) verifyOTP(ctx context.Context, otp string) (status string, success bool, err error) {
	reason := ""
	//fmt.Println("verifyOTP(", ctx, ", ", c.Args, ", ",otp,")")
	success, reason, err = c.Client.VerifyOTP(ctx, c.Args, otp)
	//fmt.Println("----------------------------------------------------")
	//fmt.Println("verifyOTP.status = ", status)
	//fmt.Println("verifyOTP.success = ", success)
	//fmt.Println("verifyOTP.err = ", err)
	//fmt.Println("----------------------------------------------------")
	if success {
		status = fmt.Sprintf("\nOTP validation successful. Git operations are now allowed.\n")
	} else {
		if err != nil {
			status = fmt.Sprintf("\nOTP validation failed.\n%v\n", err)
		} else {
			status = fmt.Sprintf("\nOTP validation failed.\n%v\n", reason)
		}
	}

	err = nil

	return
}

func (c *Command) pushAuth(ctx context.Context) (status string, success bool, err error) {
	//fmt.Println("---------------------------------------")
	reason := ""
	//fmt.Println(c.Args)
	success, reason, err = c.Client.PushAuth(ctx, c.Args)
	//fmt.Println("pushAuth.reason = ", reason)
	//fmt.Println("pushAuth.success = ", success)
	//fmt.Println("pushAuth.err = ", err)
	//fmt.Println("---------------------------------------")
	if success {
		status = fmt.Sprintf("\nPush OTP validation successful. Git operations are now allowed.\n")
	} else {
		if err != nil {
			status = fmt.Sprintf("\nPush OTP validation failed.\n%v\n", err)
		} else {
			status = fmt.Sprintf("\nPush OTP validation failed.\n%v\n", reason)
		}
	}

	return
}
