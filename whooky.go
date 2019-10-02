/* This is an example of an AWS lambda function handler triggered by a
   Stripe webhook event to send you an email reminder about an order
   payment. Modify it to suit your needs but don't do anything that
   takes more than ten seconds because it's meant to run between the
   times your customer makes a payment and Stripe redirects him to
   your thank-you page.

   copyright (c) 2019 Dennis Furey

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>. */

package main

import (
	"fmt"
	"bytes"
	"errors"
	"strings"
	"net/http"
	"io/ioutil"
	"net/textproto"
	"mime/multipart"
	"github.com/stripe/stripe-go"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stripe/stripe-go/webhook"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
)


const (
	secret               = "whsec_YOURSTRIPEWEBHOOKSECRET"         // get this from the Stripe dashboard
	stripe_private_test  = "sk_test_YOURSTRIPETESTAPIKEY"
	stripe_private_live  = "sk_live_YOURSTRIPELIVEAPIKEY"
	sender               = "order_notifier_bot@yourdomain.com"     // must be validated in advance with SES
	recipient            = "you@youremailaddress.com"
	subject              = "order"
	region               = "us-west-2"     // or wherever this is hosted
)



func acknowledgment (body string) (events.APIGatewayProxyResponse, error) {

	// Return a short message that will be visible in the stripe dashboard.

	result := events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{ "Content-Type": "text/html" },
		IsBase64Encoded: false,
		Body: body,
	}
	return result, nil
}



func checkout_session_retrieval (request events.APIGatewayProxyRequest) (stripe.CheckoutSession, error) {

	// Retrieve the stripe checkout session from the webhook payload body.

	result := stripe.CheckoutSession{}
	signature, present := request.Headers["Stripe-Signature"]
	if ! present {
		return result, errors.New ("missing stripe header")
	}
	event, err := webhook.ConstructEvent([]byte(request.Body), signature, secret)  // check that it's really from Stripe
	if err != nil {
		return result, err
	}
	if event.Type != "checkout.session.completed" {               // as specified for this webhook in the Stripe dashboard
		return result, errors.New ("unexpected event type")
	}
	err = result.UnmarshalJSON(event.Data.Raw)
	return result, err
}



func customer_retrieval (id string) (stripe.Customer, error) {

	// Retrieve a stripe customer object by its id.

	result := stripe.Customer{}
	request, err := http.NewRequest("GET", "https://api.stripe.com/v1/customers/" + id, nil)
	if err != nil {
		return result, err
	}
	request.SetBasicAuth(stripe_private_test,"")  // switch to the live key when you'e ready to go live
	client := &http.Client{}
	response, err := client.Do(request)
	defer response.Body.Close()
	if err != nil {
		return result, err
	}
	if response.StatusCode != 200 {
		return result, errors.New ("unknown customer")
	}
	response_json, err := ioutil.ReadAll(response.Body)
	err = result.UnmarshalJSON(response_json)
	return result, err
}




func payment_intent_retrieval (id string) (stripe.PaymentIntent, error) {

	// Retrieve a stripe payment intent by its id.

	result := stripe.PaymentIntent{}
	request, err := http.NewRequest("GET", "https://api.stripe.com/v1/payment_intents/" + id, nil)
	if err != nil {
		return result, err
	}
	request.SetBasicAuth(stripe_private_test,"")  // switch to the live key when you'e ready to go live
	client := &http.Client{}
	response, err := client.Do(request)
	defer response.Body.Close()
	if err != nil {
		return result, err
	}
	if response.StatusCode != 200 {
		return result, errors.New ("couldn't retrieve payment intent")
	}
	response_json, err := ioutil.ReadAll(response.Body)
	err = result.UnmarshalJSON(response_json)
	return result, err
}





func email_body (checkout_session stripe.CheckoutSession, customer stripe.Customer, payment_intent stripe.PaymentIntent, message *bytes.Buffer) (*multipart.Writer, error) {

	// Initialize and return a multipart message writer associated with
	// the given buffer after writing the first part of it, which will
	// be made to contain the text portion of the email derived from
	// the stripe checkout session.

	var payment_url string

	result := multipart.NewWriter (message)
	header := textproto.MIMEHeader{
		"Content-Type": {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	}
	amount := float64(payment_intent.Amount) / 100.0
	company, _ := payment_intent.Metadata["company"]               // Stripe metadata can be defined as whatever you want by
	quantity, _ := payment_intent.Metadata["quantity"]             // the code that creates the checkout session
	courier_name, _ := payment_intent.Metadata["courier_name"]
	payment_link, _ := payment_intent.Metadata["payment_link"]     // url to pay the courier previously stored in hex format
	_, _ = fmt.Sscanf(payment_link, "%x", &payment_url)
	body, _ := result.CreatePart (header)
	_, _ = fmt.Fprintf (body,"checkout session ID: %s\n\n",checkout_session.ID)
	if checkout_session.ClientReferenceID != "" {
		_, _ = fmt.Fprintf (body,"customer reference: %s\n\n",checkout_session.ClientReferenceID)
	}
	_, _ = fmt.Fprintf (body, "email: %s\n\n",customer.Email)
	_, _ = fmt.Fprintf (body, "phone: %s\n\n",customer.Shipping.Phone)
	if quantity == "1" {
		_, _ = fmt.Fprintf (body,"order of 1 item at £%.2f to \n\n", amount)
	} else {
		_, _ = fmt.Fprintf (body,"order of %s items at £%.2f to \n\n", quantity, amount)
	}
	if company != "" {
		_, _ = fmt.Fprintf (body,"%s\n%s\n", customer.Shipping.Name, company)
	} else {
		_, _ = fmt.Fprintf (body,"%s\n", customer.Shipping.Name)
	}
	if	customer.Shipping.Address.Line2 == "" {
		_, _ = fmt.Fprintf (
			body,
			strings.Repeat("%s\n",5) + "\n",
			customer.Shipping.Address.Line1,
			customer.Shipping.Address.City,
			customer.Shipping.Address.State,
			customer.Shipping.Address.PostalCode,
			customer.Shipping.Address.Country )
	} else {
		_, _ = fmt.Fprintf (
			body,
			strings.Repeat("%s\n",6) + "\n",
			customer.Shipping.Address.Line1,
			customer.Shipping.Address.Line2,
			customer.Shipping.Address.City,
			customer.Shipping.Address.State,
			customer.Shipping.Address.PostalCode,
			customer.Shipping.Address.Country )
	}
	_, _ = fmt.Fprintf (body, "to ship by %s payable at\n\n%s\n", courier_name, payment_url )
	return result, nil
}






func unsendable (header []byte, message []byte) error {

	// Put the header and the message together and mail them with the
	// SES raw message API. The lambda function executing this code has
	// to have permission to use SES or it won't work.

	params := &ses.SendRawEmailInput{RawMessage: &ses.RawMessage{ Data: bytes.Join ([][]byte{header,message}, []byte{})}}
	aws_session, err := session.NewSession(&aws.Config{ Region:aws.String(region)} )
	if err == nil {
		service := ses.New(aws_session)
		_, err = service.SendRawEmail(params)
	}
	return err
}







func header_of (boundary string) []byte {

	// Return the email header, which depends on the multipart message
	// boundary. The subject has to come last or else.

	var header bytes.Buffer

	header.WriteString("MIME-Version: 1.0\n")
	header.WriteString("Content-Disposition: inline\n")
	header.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\n")
	header.WriteString("From: " + sender + "\n")
	header.WriteString("To: " + recipient + "\n")
	header.WriteString("Subject: " + subject +"\n\n")
	return header.Bytes()
}









func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// Retrieve data about and order from the stripe webhook payload
	// and send a notification of it by email.

	var message bytes.Buffer

	checkout_session, err := checkout_session_retrieval (request)
	if err != nil {
		return acknowledgment ("couldn't retrieve checkout session")
	}
	if checkout_session.Customer == nil {
		return acknowledgment ("unspecified customer")
	}
	customer, err := customer_retrieval (checkout_session.Customer.ID)
	if err != nil {
		return acknowledgment ("couldn't retrieve customer")
	}
	if checkout_session.PaymentIntent == nil {
		return acknowledgment ("unspecified payment intent")
	}
	payment_intent, err := payment_intent_retrieval (checkout_session.PaymentIntent.ID)
	if err != nil {
		return acknowledgment ("couldn't retrieve payment intent")
	}
	body, err := email_body (checkout_session, customer, payment_intent, &message)
	if (err != nil) || (body.Close () != nil) {
		return acknowledgment ("couldn't compose email")
	}
	if unsendable (header_of (body.Boundary ()), message.Bytes ()) != nil {
		return acknowledgment ("couldn't send email")
	}
	return acknowledgment("ok")
}







func main() {
	lambda.Start(handler)
}
