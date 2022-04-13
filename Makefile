all:
	docker build -t public.ecr.aws/q1p8v8z2/goproxy-s3 .
	docker push public.ecr.aws/q1p8v8z2/goproxy-s3

login:
	aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws/q1p8v8z2