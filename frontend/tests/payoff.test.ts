// Run with: node --experimental-strip-types --test tests/payoff.test.ts
// (Node 22.6+). Checks the payoff sums against hand calculations.
import assert from "node:assert/strict";
import { test } from "node:test";
import { pnlAt, summarize, type PayoffLeg } from "../src/utils/payoff.ts";

const call = (side: "buy" | "sell", strike: number, price: number, quantity = 1): PayoffLeg => ({
	type: "call", side, strike, price, quantity, contractSize: 1000,
});
const put = (side: "buy" | "sell", strike: number, price: number, quantity = 1): PayoffLeg => ({
	type: "put", side, strike, price, quantity, contractSize: 1000,
});
const shares = (side: "buy" | "sell", price: number, quantity: number): PayoffLeg => ({
	type: "underlying", side, strike: null, price, quantity, contractSize: null,
});

// Bull call spread: buy call 2000 @150, sell call 2100 @80. Debit 70 x 1000 = 70,000.
const bull = [call("buy", 2000, 150), call("sell", 2100, 80)];

test("bull call spread by hand", () => {
	const s = summarize(bull, 2000);
	assert.equal(s.netPremium, 70_000);
	assert.equal(s.maxProfit, 30_000); // (100 - 70) x 1000
	assert.equal(s.maxLoss, 70_000);
	assert.deepEqual(s.breakEvens, [2070]); // 2000 + 70
	assert.equal(pnlAt(bull, 1900), -70_000);
	assert.equal(pnlAt(bull, 2050), -20_000); // (50 - 70) x 1000
	assert.equal(pnlAt(bull, 2300), 30_000);
});

test("payoff curve: 41 points 50%..150% plus the strikes, ascending", () => {
	const s = summarize(bull, 2000);
	const xs = s.payoff.map((p) => p.underlyingPrice);
	assert.deepEqual(xs, [...xs].sort((a, b) => a - b));
	assert.equal(xs[0], 1000);
	assert.equal(xs[xs.length - 1], 3000);
	assert.ok(xs.includes(2100) && xs.includes(2000));
	assert.equal(xs.length, 41); // 2000 and 2100 already lie on the 41-point grid
});

test("long call: unlimited profit, loss is the premium", () => {
	const s = summarize([call("buy", 2000, 150)], 2000);
	assert.equal(s.maxProfit, null);
	assert.equal(s.maxLoss, 150_000);
	assert.deepEqual(s.breakEvens, [2150]);
});

test("long straddle has two break-evens", () => {
	const s = summarize([call("buy", 2000, 100), put("buy", 2000, 90)], 2000);
	assert.equal(s.netPremium, 190_000);
	assert.deepEqual(s.breakEvens, [1810, 2190]);
	assert.equal(s.maxProfit, null);
	assert.equal(s.maxLoss, 190_000);
});

test("covered call: shares plus a short call", () => {
	const legs = [shares("buy", 2000, 1000), call("sell", 2100, 60)];
	const s = summarize(legs, 2000);
	assert.equal(s.netPremium, 2_000_000 - 60_000);
	assert.equal(s.maxProfit, 160_000); // (2100 - 2000 + 60) x 1000
	assert.equal(s.maxLoss, 1_940_000); // shares to zero, keep the 60 premium
	assert.deepEqual(s.breakEvens, [1940]);
});

test("short put: profit is the premium, loss is the strike minus it", () => {
	const s = summarize([put("sell", 2000, 90)], 2000);
	assert.equal(s.maxProfit, 90_000);
	assert.equal(s.maxLoss, (2000 - 90) * 1000);
});
