# FROM python:latest
FROM --platform=linux/amd64 python:latest

# Set the working directory to /app
WORKDIR /app

COPY main.py /app

RUN pip install requests redis

CMD ["python", "main.py"]