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

func (c *Command) Execute1(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)

	// config.GetHTTPClient isn't thread-safe so save Client in struct for concurrency
	// workaround until #518 is fixed
	var err error
	c.Client, err = twofactorverify.NewClient(c.Config)

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
			fmt.Println("-------------")
			fmt.Println("pushAuth.status = ", status)
			fmt.Println("pushAuth.success = ", success)
			fmt.Println("pushAuth.err = ", err)
			fmt.Println("-------------")
			ctxlog.WithError(err).Info("twofactorverify: execute: OTP verified")
			verify <- Result{Error: err, Status: status, Success: success}
		}
	}()


	for {
		select {
		case res := <-verify: // manual OTP
			fmt.Println("-------------")
			fmt.Println("verify.res = ", res)
			fmt.Println("-------------")
			if res.Status == "cancelled" {
				// verify cancelled; don't print anything
			} else if res.Status == "" {
				// channel closed; don't print anything
			} else {
				fmt.Fprint(c.ReadWriter.Out, res.Status)
				return nil
			}
		case res := <-pushauth: // push
			fmt.Println("-------------")
			fmt.Println("pushauth.res = ", res)
			fmt.Println("-------------")
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

	// Create timeout context
	// TODO: make timeout configurable
	const ctxTimeout = 30
	myctx, mycancel := context.WithCancel(ctx)
	defer mycancel()
	//defer cancelTimeout()
	//
	//// Create result channel
	//resultC := make(chan Result)

	c.processCmd(myctx, mycancel)

	//
	for {
		fmt.Println("for")
		fmt.Println(myctx)
		select {
			case <- myctx.Done():
				fmt.Println("myctx.Done")
			//case resPush := <-pushAuth.D:
			//	fmt.Println("resPush => ", resPush.Status)
			//	//if resPush.Status == "cancelled" {
			//	//	// request cancelled; don't print anything
			//	//} else if resPush.Status == "" {
			//	//	// channel closed; don't print anything
			//	//} else {
			//	//	fmt.Fprint(c.ReadWriter.Out, resPush.Status)
			//	//	return nil
			//	//}
			//case resOtp := <-otpAuth:
			//	fmt.Println("otpAuth => ", resOtp)
			//	//if resOtp.Status == "cancelled" {
			//	//	// request cancelled; don't print anything
			//	//} else if resOtp.Status == "" {
			//	//	// channel closed; don't print anything
			//	//} else {
			//	//	fmt.Fprint(c.ReadWriter.Out, resOtp.Status)
			//	//	return nil
			//	//}

			case <-myctx.Done(): // push timed out
				fmt.Fprint(c.ReadWriter.Out, "\nOTP verification timed out\n")
				return nil
			default:
				fmt.Println("myctx == ", myctx)

		}
	}

	return nil
}

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

func (c *Command) processCmd(ctx context.Context, cancelTimeout context.CancelFunc) (status string, success bool, err error) {
	ctxlog := log.ContextLogger(ctx)
	// Background push notification with timeout
	pushAuth := make(chan Result)
	go func() {
		defer close(pushAuth)
		status, success, err := c.pushAuth(ctx)
		fmt.Println("-------------")
		fmt.Println("pushAuth.status = ", status)
		fmt.Println("pushAuth.success = ", success)
		fmt.Println("pushAuth.err = ", err)
		fmt.Println("-------------")

		select {
		case <-ctx.Done(): // push cancelled by manual OTP
			fmt.Println("pushAuth.func.timeoutCtx.Done()")
			pushAuth <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			fmt.Println("pushAuth.func.default")
			pushAuth <- Result{Error: err, Status: status, Success: success}
			cancelTimeout()
		}
	}()

	// Also allow manual OTP entry while waiting for push, with same timeout as push
	otpAuth := make(chan Result)
	go func() {
		defer close(otpAuth)
		fmt.Println("twofactorverify: execute: waiting for user input")
		otpAnswer := c.getOTP(ctx)
		fmt.Println("otpAnswer = ", otpAnswer)

		select {
		case <-ctx.Done(): // manual OTP cancelled by push
			fmt.Println("otpAuth.func.timeoutCtx.Done()")
			otpAuth <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			fmt.Println("otpAuth.func.timeoutCtx.default")
			cancelTimeout()
			fmt.Println("twofactorverify: execute: verifying entered OTP")
			status, success, err := c.verifyOTP(ctx, otpAnswer)
			ctxlog.WithError(err).Info("twofactorverify: execute: OTP verified")
			otpAuth <- Result{Error: err, Status: status, Success: success}
		}
	}()
	for {
		select {
			case pres := <- pushAuth:
				fmt.Println("-------------")
				fmt.Println("pushAuth = ", pres)
				fmt.Println("-------------")
				if len(pres.Status) > 0 {
					fmt.Println("-------------")
					fmt.Println("pushAuth = ", pres.Status)
					fmt.Println("-------------")
				}
			case ores := <- otpAuth:
				fmt.Println("-------------")
				fmt.Println("otpAuth = ", ores)
				fmt.Println("-------------")
				if len(ores.Status) > 0 {
					fmt.Println("-------------")
					fmt.Println("otpAuth = ", ores.Status)
					fmt.Println("-------------")

				}
		}
	}
	return
}





func (c *Command) verifyOTP(ctx context.Context, otp string) (status string, success bool, err error) {
	reason := ""
	fmt.Println("verifyOTP(ctx, ",otp,")")
	success, reason, err = c.Client.VerifyOTP(ctx, c.Args, otp)
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
	fmt.Println("---------------------------------------")
	reason := ""
	fmt.Println(c.Args)
	success, reason, err = c.Client.PushAuth(ctx, c.Args)
	fmt.Println("pushAuth.reason = ", reason)
	fmt.Println("pushAuth.success = ", success)
	fmt.Println("pushAuth.err = ", err)
	fmt.Println("---------------------------------------")
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
