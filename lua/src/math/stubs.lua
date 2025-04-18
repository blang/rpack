--- Math library.
-- This library is preloaded into the global namespace.
-- Functions such as math.abs, math.sin, math.sqrt etc are available.
-- @module math

-- A constant representing the ratio of a circle's circumference to its diameter.
math.pi = math.pi  -- (The actual value is set in Go)

-- A constant representing a huge number (maximum float value).
math.huge = math.huge  -- (The actual value is set in Go)

---
-- Returns the absolute value of a number.
--
-- @param x number The number.
-- @return number The absolute value of x.
function math.abs(x)
end

---
-- Returns the arc cosine of x (in radians).
--
-- @param x number A number.
-- @return number The arc cosine.
function math.acos(x)
end

---
-- Returns the arc sine of x (in radians).
--
-- @param x number A number.
-- @return number The arc sine.
function math.asin(x)
end

---
-- Returns the arc tangent of x (in radians).
--
-- @param x number A number.
-- @return number The arc tangent.
function math.atan(x)
end

---
-- Returns the arc tangent of y/x (in radians), using two arguments.
--
-- @param y number The numerator.
-- @param x number The denominator.
-- @return number The arc tangent of y/x.
function math.atan2(y, x)
end

---
-- Returns x rounded up to the nearest integer.
--
-- @param x number A number.
-- @return number The smallest integer greater than or equal to x.
function math.ceil(x)
end

---
-- Returns the cosine of x (in radians).
--
-- @param x number A number.
-- @return number The cosine value.
function math.cos(x)
end

---
-- Returns the hyperbolic cosine of x.
--
-- @param x number A number.
-- @return number The hyperbolic cosine.
function math.cosh(x)
end

---
-- Converts an angle in radians to degrees.
--
-- @param x number An angle in radians.
-- @return number The angle in degrees.
function math.deg(x)
end

---
-- Returns e raised to the power of x.
--
-- @param x number A number.
-- @return number The computed exponential value.
function math.exp(x)
end

---
-- Returns the largest integer less than or equal to x.
--
-- @param x number A number.
-- @return number The floor value of x.
function math.floor(x)
end

---
-- Returns the remainder of the division of x by y.
--
-- @param x number The dividend.
-- @param y number The divisor.
-- @return number The remainder.
function math.fmod(x, y)
end

---
-- Returns the mantissa and exponent of x as two numbers, such that x = mantissa * 2^exponent.
--
-- @param x number A number.
-- @return number mantissa.
-- @return number exponent.
function math.frexp(x)
end

---
-- Returns x multiplied by 2 raised to the power of exp.
--
-- @param x number A number.
-- @param exp number An exponent.
-- @return number The result of x * 2^exp.
function math.ldexp(x, exp)
end

---
-- Returns the natural logarithm of x.
--
-- @param x number A number.
-- @return number The natural logarithm of x.
function math.log(x)
end

---
-- Returns the base-10 logarithm of x.
--
-- @param x number A number.
-- @return number The base-10 logarithm.
function math.log10(x)
end

---
-- Returns the maximum value among the given arguments.
--
-- @param ... number One or more numbers.
-- @return number The largest value.
function math.max(...)
end

---
-- Returns the minimum value among the given arguments.
--
-- @param ... number One or more numbers.
-- @return number The smallest value.
function math.min(...)
end

---
-- Returns the modulo of x by y.
--
-- @param x number The dividend.
-- @param y number The divisor.
-- @return number The computed modulo.
function math.mod(x, y)
end

---
-- Returns the integral and fractional parts of x.
--
-- @param x number A number.
-- @return number The integer part.
-- @return number The fractional part.
function math.modf(x)
end

---
-- Returns x raised to the power y.
--
-- @param x number The base.
-- @param y number The exponent.
-- @return number The computed power.
function math.pow(x, y)
end

---
-- Converts an angle in degrees to radians.
--
-- @param x number An angle in degrees.
-- @return number The angle in radians.
function math.rad(x)
end

---
-- Returns a pseudo-random number.
-- When called without arguments, returns a float in the range [0,1).
-- When called with one integer argument n, returns an integer in the range [1,n].
-- When called with two integer arguments (min, max), returns an integer in the range [min, max].
--
-- @param a? number The upper or lower bound.
-- @param b? number The upper bound when two arguments are provided.
-- @return number A random number.
function math.random(a, b)
end

---
-- Sets the seed for the pseudo-random number generator.
--
-- @param seed number The seed value.
function math.randomseed(seed)
end

---
-- Returns the sine of x (in radians).
--
-- @param x number A number.
-- @return number The sine value.
function math.sin(x)
end

---
-- Returns the hyperbolic sine of x.
--
-- @param x number A number.
-- @return number The hyperbolic sine.
function math.sinh(x)
end

---
-- Returns the square root of x.
--
-- @param x number A non-negative number.
-- @return number The square root of x.
function math.sqrt(x)
end

---
-- Returns the tangent of x (in radians).
--
-- @param x number A number.
-- @return number The tangent value.
function math.tan(x)
end

---
-- Returns the hyperbolic tangent of x.
--
-- @param x number A number.
-- @return number The hyperbolic tangent.
function math.tanh(x)
end
