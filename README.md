# whooky -- Golang AWS Lambda Handler example for Stripe Webhooks

Stripe lets you set up a webhook to signal any event relevant to your
account, such as a customer making a payment. This repo contains an
example of a Go package that implements such a webhook as an AWS
Lambda function. Based on information extracted from the webhook
payload, it emails the number of items ordered and the customer's name
and shipping address to you, the person whose job it is to fulfill the
order. If your site has previously collected the shipping address and
stored it with Stripe, you can get away with not running your own
database.

This package is not a complete project but should give you a head
start at handling this part of the order flow in your own project. I
use it myself as part of a small ecommerce web site custom written to
avoid wasting money on ecommerce platforms like Shopify. It uses the
raw message API for sending email in case you want to modify it for
binary attachments. See my lumberjack repo for an example of binary
email attachments in Go via SES.

Watch out for a cyclic dependence when setting this up. In the code,
you need to define a constant containing the secret you get from the
Stripe dashboard when configuring the webhook, but you need to deploy
the code as a Lambda function for Amazon to assign it the url you tell
to Stripe in the webhook configuration. Non-geniuses at devops can do
what I did by deploying an incomplete version first just to get the
url, and then update it.

Don't blame me if you get pwned by skipping authentication via the
webhook secret, and don't trust any email to your order fulfillment
address without checking your bank statement.
