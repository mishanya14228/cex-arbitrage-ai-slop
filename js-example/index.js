require('dotenv').config({path: "../.env"}); // Initialize dotenv to load environment variables from .env file
const amqp = require('amqplib');

async function consumeMessages() {
    const rabbitUser = process.env.RABBITMQ_DEFAULT_USER || 'guest';
    const rabbitPass = process.env.RABBITMQ_DEFAULT_PASS || 'guest';
    const rabbitHost = process.env.RABBITMQ_HOST || 'localhost';
    const rabbitMQURL = `amqp://${rabbitUser}:${rabbitPass}@${rabbitHost}:5672/`;
    const queueName = 'arbitrage_event';

    console.log(`Attempting to connect to RabbitMQ at ${rabbitMQURL}`);

    try {
        const connection = await amqp.connect(rabbitMQURL);
        const channel = await connection.createChannel();

        await channel.assertQueue(queueName, {
            durable: false
        });

        console.log(`[*] Waiting for messages in ${queueName}. To exit press CTRL+C`);

        channel.consume(queueName, (msg) => {
            if (msg.content) {
                console.log("[x] Received %s", msg.content.toString());
                // Acknowledge the message to remove it from the queue
                channel.ack(msg);
            }
        }, {
            noAck: false // We will manually acknowledge messages
        });

    } catch (error) {
        console.error("Failed to consume messages:", error);
        process.exit(1);
    }
}

consumeMessages();