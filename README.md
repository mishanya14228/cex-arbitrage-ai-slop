# how to run this ai slop
cp .env.example .env
docker compose up --build -d
cd js-example
npm i
node index.js
